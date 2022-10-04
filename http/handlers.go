package main

import (
	"context"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

//go:embed static/index.html
var indexPage string

const (
	invalidHashError = "'%s' is not a valid short path."
	invalidURLError  = "'%s' is not a valid URL."
)

var (
	short        = regexp.MustCompile(`[a-zA-Z0-9]{8}`)
	long         = regexp.MustCompile(`https?://(?:[-\w.]|%[\da-fA-F]{2})+`)
	sessionToken = "session_token"
)

type handlers struct {
	tr      trace.Tracer
	auth    *auth
	storage Storage
	router  *mux.Router
}

func newHandlers(ctx context.Context, tr trace.Tracer, a *auth, s Storage) (*handlers, error) {
	_, span := tr.Start(ctx, "newHandlers")
	defer span.End()

	h := &handlers{
		tr:      tr,
		auth:    a,
		storage: s,
		router:  mux.NewRouter(),
	}
	h.router.HandleFunc("/", h.handleIndex).Methods(http.MethodGet)
	h.router.HandleFunc("/login", h.handleLogin).Methods(http.MethodPost)
	h.router.HandleFunc("/shorten", h.handleShorten).Methods(http.MethodPost)
	h.router.HandleFunc("/{[0-9a-fA-F]{8}}", h.handleLonger).Methods(http.MethodGet)

	return h, nil
}

type Credentials struct {
	Password string `json:"password"`
	Username string `json:"username"`
}

func (h *handlers) handleLogin(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "login")
	defer span.End()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeResponse(w, http.StatusInternalServerError, "read body failed: "+err.Error())
		span.SetAttributes(attribute.Bool("error", true))
		span.RecordError(err)
		return
	}

	var creds Credentials
	err = json.Unmarshal(body, &creds)
	if err != nil {
		writeResponse(w, http.StatusBadRequest, "cannot unmarshal body to credentials json: "+err.Error())
		span.SetAttributes(attribute.Bool("error", true))
		span.RecordError(err)
		return
	}

	token, expireAt, err := h.auth.Login(ctx, creds.Username, creds.Password)
	if err != nil {
		writeResponse(w, http.StatusBadRequest, "authenticate failed: "+err.Error())
		span.SetAttributes(attribute.Bool("error", true))
		span.RecordError(err)
		return
	}

	span.SetAttributes()

	http.SetCookie(w, &http.Cookie{
		Name:    sessionToken,
		Value:   token,
		Expires: expireAt,
	})
	w.WriteHeader(http.StatusOK)
}

func (h *handlers) handleIndex(w http.ResponseWriter, r *http.Request) {
	_, span := h.tr.Start(r.Context(), "index")
	defer span.End()

	w.Header().Set("Content-Type", "text/html")
	writeResponse(w, http.StatusOK, indexPage)
}

func writeResponse(w http.ResponseWriter, statusCode int, body string) {
	w.WriteHeader(statusCode)
	_, _ = w.Write([]byte(body))
}

func isShortCorrect(link string) bool {
	return short.FindStringIndex(link) != nil
}

func isLongCorrect(link string) bool {
	return long.FindStringIndex(link) != nil
}

func getHash(s []byte) (string, error) {
	hasher := fnv.New32a()
	_, err := hasher.Write(s)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func (h *handlers) handleShorten(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "shorten")
	defer span.End()

	if c, err := r.Cookie(sessionToken); err != nil {
		writeResponse(w, http.StatusUnauthorized, "session token expected")
		span.SetAttributes(attribute.Bool("error", true))
		span.RecordError(err)
		return
	} else if err = h.auth.Validate(ctx, c.Value); err != nil {
		writeResponse(w, http.StatusUnauthorized, err.Error())
		span.SetAttributes(attribute.Bool("error", true))
		span.RecordError(err)
		return
	}

	url, err := io.ReadAll(r.Body)
	if err != nil {
		writeResponse(w, http.StatusInternalServerError, err.Error())
		span.SetAttributes(attribute.Bool("error", true))
		span.RecordError(err)
		return
	}

	if !isLongCorrect(string(url)) {
		err = fmt.Errorf(fmt.Sprintf(invalidURLError, url))
		writeResponse(w, http.StatusBadRequest, err.Error())
		span.SetAttributes(attribute.Bool("error", true))
		span.RecordError(err)
		return
	}

	hash, err := getHash(url)
	if err != nil {
		writeResponse(w, http.StatusInternalServerError, err.Error())
		span.SetAttributes(attribute.Bool("error", true))
		span.RecordError(err)
		return
	}

	err = h.storage.Put(ctx, string(url), hash)
	if err != nil {
		writeResponse(w, http.StatusInternalServerError, err.Error())
		span.SetAttributes(attribute.Bool("error", true))
		span.RecordError(err)
		return
	}

	w.Header().Set("Content-Type", "application/text")
	writeResponse(w, http.StatusOK, hash)
}

func (h *handlers) handleLonger(w http.ResponseWriter, r *http.Request) {
	ctx, span := h.tr.Start(r.Context(), "longer")
	defer span.End()

	path := strings.Split(r.URL.Path, "/")
	if !isShortCorrect(path[len(path)-1]) {
		err := fmt.Errorf(invalidHashError, path[len(path)-1])
		writeResponse(w, http.StatusBadRequest, err.Error())
		span.SetAttributes(attribute.Bool("error", true))
		span.RecordError(err)
		return
	}

	url, err := h.storage.Get(ctx, path[len(path)-1])
	if err != nil {
		writeResponse(w, http.StatusInternalServerError, err.Error())
		span.SetAttributes(attribute.Bool("error", true))
		span.RecordError(err)
		return
	}

	http.Redirect(w, r, url, http.StatusSeeOther)
}

func (h *handlers) run(ctx context.Context, port int) {
	ctx, span := h.tr.Start(ctx, "run")
	defer span.End()

	server := &http.Server{
		Addr:    ":" + strconv.Itoa(port),
		Handler: h.router,
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)

	go func() {
		if err := server.ListenAndServe(); err != nil {
			close(ch)
		}
	}()

	fmt.Printf("Start URL shortener on port %d...\n", port)

	for s := range ch {
		fmt.Println("shutdown...")
		span.AddEvent("received signal", trace.WithAttributes(
			attribute.String("signal", s.String()),
		))
		_ = server.Shutdown(ctx)
	}
}
