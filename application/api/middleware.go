package api

import (
	"errors"
	"net/http"

	"github.com/0xAtelerix/sdk/gosdk/rpc"
	"github.com/rs/zerolog"
)

// ErrNilRequestBody is returned when the request body is nil
var ErrNilRequestBody = errors.New("request body is nil")

type ExampleMiddleware struct {
	log zerolog.Logger
}

func NewExampleMiddleware(log zerolog.Logger) *ExampleMiddleware {
	return &ExampleMiddleware{
		log: log,
	}
}

func (e *ExampleMiddleware) ProcessRequest(
	_ http.ResponseWriter,
	r *http.Request,
) error {
	e.log.Info().Msgf("Processing request: %s %s", r.Method, r.URL.Path)

	if r.Body == nil { // Dummy check for example
		return ErrNilRequestBody
	}

	return nil
}

func (e *ExampleMiddleware) ProcessResponse(
	_ http.ResponseWriter,
	_ *http.Request,
	response rpc.JSONRPCResponse,
) error {
	e.log.Info().Msgf("Processing response ID: %v", response.ID)

	if response.Error != nil { // Dummy check for example
		e.log.Error().Msgf("Error in response: %v", response.Error)

		return response.Error
	}

	e.log.Info().Msgf("Response result: %v", response.Result)

	return nil
}
