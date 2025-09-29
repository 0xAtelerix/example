package api

import (
	"net/http"

	"github.com/0xAtelerix/sdk/gosdk/rpc"
	"github.com/rs/zerolog"
)

type ExampleMiddleware struct {
	log zerolog.Logger
}

func NewExampleMiddleware(log zerolog.Logger) *ExampleMiddleware {
	return &ExampleMiddleware{
		log: log,
	}
}

func (e *ExampleMiddleware) ProcessRequest(w http.ResponseWriter, r *http.Request) error {
	e.log.Info().Msgf("Processing request: %s %s", r.Method, r.URL.Path)
	return nil
}

func (e *ExampleMiddleware) ProcessResponse(w http.ResponseWriter, r *http.Request, response rpc.JSONRPCResponse) error {
	e.log.Info().Msgf("Processing response ID: %v", response.ID)
	if response.Error != nil {
		e.log.Error().Msgf("Error in response: %v", response.Error)
	} else {
		e.log.Info().Msgf("Response result: %v", response.Result)
	}
	return nil
}
