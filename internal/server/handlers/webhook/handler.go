// Package webhook exposes the OmniDesk webhook endpoint.
package webhook

import (
	"crypto/subtle"
	"fmt"

	"github.com/go-core-fx/fiberfx/handler"
	"github.com/go-core-fx/fiberfx/validation"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/keyauth"
	"github.com/support-loop/backend/internal/ingest"
	"github.com/support-loop/backend/internal/worker"
)

// Handler exposes the webhook endpoint over HTTP.
type Handler struct {
	handler.Base

	service *ingest.Service
	config  ingest.Config
}

// New creates a new webhook HTTP handler.
func New(
	service *ingest.Service,
	config ingest.Config,
	validator *validator.Validate,
) handler.Handler {
	return &Handler{
		Base: handler.Base{
			Validator: validator,
		},
		service: service,
		config:  config,
	}
}

// Register registers the webhook routes. The group owns its error handler:
// errors from the routes are mapped to HTTP responses here, at the same
// level as the handlers that return them.
func (h *Handler) Register(r fiber.Router) {
	g := r.Group("/webhooks")
	g.Use(errorHandler)
	g.Post("/omnidesk", h.auth(), validation.DecorateWithBodyEx(h.Validator, h.post))
}

// auth returns the Fiber keyauth middleware protecting the webhook. The
// shared secret travels in the fixed Authorization header; the value format
// (raw token vs Bearer vs Basic) is unverified (W5), so the raw header value
// is compared constant-time against the configured secret.
func (h *Handler) auth() fiber.Handler {
	return keyauth.New(keyauth.Config{
		KeyLookup:  "header:" + h.config.SecretHeader,
		AuthScheme: "",
		Validator: func(_ *fiber.Ctx, key string) (bool, error) {
			return subtle.ConstantTimeCompare([]byte(key), []byte(h.config.Secret)) == 1, nil
		},
	})
}

//	@Summary		Receive OmniDesk webhook
//	@Description	Accepts an authored webhook event, deduplicates it and enqueues it for processing
//	@Tags			webhook
//	@Accept			json
//	@Param			request	body	Request	true	"Webhook payload"
//	@Success		200
//	@Failure		400	{object}	fiberfx.ErrorResponse
//	@Failure		403	{object}	fiberfx.ErrorResponse
//	@Failure		500	{object}	fiberfx.ErrorResponse
//	@Router			/webhooks/omnidesk [post]
//
// Post handles the webhook. The body is parsed and validated by
// DecorateWithBodyEx; the service deduplicates and enqueues. Domain errors
// are converted to HTTP responses by this group's error handler.
func (h *Handler) post(c *fiber.Ctx, req *Request) error {
	if _, err := h.service.Process(c.Context(), worker.Event{
		EventType: req.EventType,
		CaseID:    int64(req.CaseID),
		Payload:   c.Body(),
	}); err != nil {
		return fmt.Errorf("process webhook event: %w", err)
	}
	return nil
}
