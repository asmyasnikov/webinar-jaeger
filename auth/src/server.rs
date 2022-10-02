use auth::auth_server::{Auth, AuthServer};
use auth::{LoginRequest, LoginResponse, ValidateRequest, ValidateResponse};
use once_cell::sync::Lazy;
use opentelemetry::global;
use opentelemetry::trace::TraceError;
use opentelemetry::{
    propagation::Extractor,
    trace::{Span, Tracer},
    KeyValue,
};
use prost_types::Timestamp;
use std::collections::HashMap;
use std::ops::Add;
use std::time::{Duration, SystemTime};
use tonic::{transport::Server, Request, Response, Status};
use uuid::Uuid;
use r2d2_redis::{r2d2, redis::Commands, RedisConnectionManager};

const APPLICATION_ID: &str = "auth";

pub mod auth {
    tonic::include_proto!("auth");
}

struct User<'a> {
    name: &'a str,
    password: &'a str,
}

const USERS: &'static [User] = &[
    User {
        name: "root",
        password: "admin",
    },
    User {
        name: "user",
        password: "user",
    },
];

static PASSWORDS: Lazy<HashMap<String, String>> = Lazy::new(|| {
    let mut map = HashMap::new();

    for user in USERS {
        map.insert(user.name.to_owned(), user.password.to_owned());
    }

    map
});

struct MetadataMap<'a>(&'a tonic::metadata::MetadataMap);

impl<'a> Extractor for MetadataMap<'a> {
    /// Get a value for a key from the MetadataMap.  If the value can't be converted to &str, returns None
    fn get(&self, key: &str) -> Option<&str> {
        self.0.get(key).and_then(|metadata| metadata.to_str().ok())
    }

    /// Collect all the keys from the MetadataMap.
    fn keys(&self) -> Vec<&str> {
        self.0
            .keys()
            .map(|key| match key {
                tonic::metadata::KeyRef::Ascii(v) => v.as_str(),
                tonic::metadata::KeyRef::Binary(v) => v.as_str(),
            })
            .collect::<Vec<_>>()
    }
}

pub struct AuthService {
    session_id: String,
    pool: r2d2::Pool<RedisConnectionManager>,
}

#[tonic::async_trait]
impl Auth for AuthService {
    async fn login(
        &self,
        request: Request<LoginRequest>,
    ) -> Result<Response<LoginResponse>, Status> {
        let parent_cx =
            global::get_text_map_propagator(|prop| prop.extract(&MetadataMap(request.metadata())));
        let mut span = global::tracer(APPLICATION_ID).start_with_context("login", &parent_cx);
        span.set_attribute(KeyValue::new("request", format!("{:?}", request)));

        let req = request.into_inner();

        if !PASSWORDS.contains_key(&req.user) {
            let err = Status::unauthenticated("user not found");
            span.set_attribute(KeyValue::new("error", true));
            span.record_error(&err);
            return Err(err);
        }

        span.add_event("user well known", vec![]);

        if PASSWORDS[&req.user] != req.password {
            let err = Status::unauthenticated("wrong password");
            span.set_attribute(KeyValue::new("error", true));
            span.record_error(&err);
            return Err(err);
        }

        let token = Uuid::new_v4().hyphenated().to_string();

        let mut conn = self.pool.get().unwrap();

        let ttl = Duration::from_secs(60);

        let _: () = conn.set_ex(&token, &self.session_id, ttl.as_millis() as usize).unwrap();

        let expire_at = std::option::Option::Some(Timestamp::from(SystemTime::now().add(ttl)));

        Ok(Response::new(LoginResponse { 
            token,
            expire_at,
         }))
    }
    async fn validate(
        &self,
        request: Request<ValidateRequest>,
    ) -> Result<Response<ValidateResponse>, Status> {
        let parent_cx =
            global::get_text_map_propagator(|prop| prop.extract(&MetadataMap(request.metadata())));
        let mut span = global::tracer(APPLICATION_ID).start_with_context("validate", &parent_cx);
        span.set_attribute(KeyValue::new("request", format!("{:?}", request)));

        let token = request.into_inner().token;

        let mut conn = self.pool.get().unwrap();

        match conn.get::<&std::string::String, r2d2_redis::redis::Value>(&token) {
            Ok(value) => match value {
                r2d2_redis::redis::Value::Data(session_id) => {
                    let session_id = match String::from_utf8(session_id) {
                        Ok(session_id) => session_id,
                        Err(err) => {
                            span.set_attribute(KeyValue::new("error", true));
                            span.record_error(&err);
                            return Err(Status::internal(err.to_string()));
                        }
                    };
                    if session_id != self.session_id {
                        let err = Status::unauthenticated("wrong session ID");
                        span.set_attribute(KeyValue::new("error", true));
                        span.record_error(&err);
                        Err(err)
                    } else {
                        span.add_event("token exists in redis", vec![]);
                        Ok(Response::new(ValidateResponse {}))
                    }
                }
                _ => {
                    let err = Status::unauthenticated(format!("wrong redis response: {:?}", value));
                    span.set_attribute(KeyValue::new("error", true));
                    span.record_error(&err);
                    Err(err)
                }
            },
            Err(err) => {
                let err = Status::unauthenticated(err.to_string());
                span.set_attribute(KeyValue::new("error", true));
                span.record_error(&err);
                Err(err)
            }
        }
    }
}

impl AuthService {
    fn new(pool: r2d2::Pool<RedisConnectionManager>) -> Self {
        let session_id = Uuid::new_v4().hyphenated().to_string();

        AuthService { session_id, pool }
    }
}

fn tracing_init() -> Result<impl Tracer, TraceError> {
    global::set_text_map_propagator(opentelemetry_jaeger::Propagator::new());
    opentelemetry_jaeger::new_agent_pipeline()
        .with_service_name(APPLICATION_ID)
        .install_simple()
}

fn intercept(req: Request<()>) -> Result<Request<()>, Status> {
    println!("Intercepting request: {:?}", req);

    Ok(req)
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    println!("start");
    let _tracer = tracing_init()?;
    println!("tracer initialized");
    let addr = "127.0.0.1:50051".parse()?;
    let manager = RedisConnectionManager::new("redis://127.0.0.1").unwrap();
    let pool = r2d2::Pool::builder()
        .build(manager)
        .unwrap();
    println!("redis client opened");
    let auth_service = AuthServer::with_interceptor(AuthService::new(pool), intercept);

    println!("starting server on addres {}...", addr);

    Server::builder()
        .add_service(auth_service)
        .serve(addr)
        .await?;

    println!("server started");

    Ok(())
}
