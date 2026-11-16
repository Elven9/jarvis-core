package server

import (
	"encoding/json"
	"net/http"

	"github.com/gofiber/fiber/v3"
)

// contentTypeProblem is the RFC 9457 media type for problem details.
const contentTypeProblem = "application/problem+json"

// Problem is an error answered as RFC 9457 problem details by the error handler.
// Type is always "about:blank" for now, so Title is the status phrase.
type Problem struct {
	Status int
	Detail string // shown to the client; empty for internal errors
	Err    error  // internal cause, logged but never sent
}

func (p *Problem) Error() string {
	if p.Err != nil {
		return p.Err.Error()
	}
	return p.Detail
}

func (p *Problem) Unwrap() error { return p.Err }

// newProblem returns a problem the client may read the detail of.
func newProblem(status int, detail string) *Problem {
	return &Problem{Status: status, Detail: detail}
}

// internalProblem hides err from the client and answers 500.
func internalProblem(err error) *Problem {
	return &Problem{Status: fiber.StatusInternalServerError, Err: err}
}

type problemBody struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
}

func sendProblem(c fiber.Ctx, p *Problem) error {
	body, err := json.Marshal(problemBody{
		Type:   "about:blank",
		Title:  http.StatusText(p.Status),
		Status: p.Status,
		Detail: p.Detail,
	})
	if err != nil {
		return err
	}
	c.Set(fiber.HeaderContentType, contentTypeProblem)
	return c.Status(p.Status).Send(body)
}
