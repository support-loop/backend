package webhook

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-core-fx/fiberfx/validation"
	"github.com/gofiber/fiber/v2"
	"github.com/support-loop/backend/internal/ingest"
	"github.com/support-loop/backend/internal/worker"
)

// errorHandler converts the domain errors returned by webhook handlers into
// HTTP responses. It is registered on this handler's router group only;
// errors it does not recognize are returned so fiberfx's JSON error handler
// formats them as a sanitized 500.
func errorHandler(c *fiber.Ctx) error {
	err := c.Next()
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, ingest.ErrEventNotAllowed):
		// Ignored events (unknown event type) complete successfully so
		// OmniDesk stops retrying them.
		return c.SendStatus(fiber.StatusOK)
	case errors.Is(err, worker.ErrChannelFull):
		return fiber.NewError(fiber.StatusInternalServerError, http.StatusText(fiber.StatusInternalServerError))
	case isBodyError(err):
		// Malformed bodies mean the OmniDesk rule template we authored is
		// misconfigured: a client problem, not a server fault.
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	default:
		return err
	}
}

// isBodyError reports whether err is a request-body error: the validation
// failures or JSON/parse errors returned by DecorateWithBodyEx and the DTO.
func isBodyError(err error) bool {
	var vErr validation.Errors
	var synErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	var numErr *strconv.NumError
	return errors.As(err, &vErr) ||
		errors.As(err, &synErr) ||
		errors.As(err, &typeErr) ||
		errors.As(err, &numErr)
}
