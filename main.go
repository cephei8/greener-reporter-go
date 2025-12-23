package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/urfave/cli/v3"
)

type ActionType string

const (
	actionRun    ActionType = "run"
	actionPass   ActionType = "pass"
	actionFail   ActionType = "fail"
	actionSkip   ActionType = "skip"
	actionOutput ActionType = "output"
)

const (
	ingressEndpointFlag = "ingress-endpoint"
	ingressAPIKeyFlag   = "ingress-api-key"
)

const (
	statusPass  = "pass"
	statusFail  = "fail"
	statusSkip  = "skip"
	statusError = "error"
)

type Event struct {
	Time    string     `json:"Time"`
	Action  ActionType `json:"Action"`
	Package string     `json:"Package"`
	Test    string     `json:"Test,omitempty"`
	Output  string     `json:"Output,omitempty"`
	Elapsed float64    `json:"Elapsed,omitempty"`
}

type SessionRequest struct {
	Id          string         `json:"id,omitempty"`
	Description string         `json:"description,omitempty"`
	Baggage     map[string]any `json:"baggage,omitempty"`
}

type SessionResponse struct {
	Id string `json:"id"`
}

type TestcaseRequest struct {
	SessionId         string         `json:"sessionId"`
	TestcaseName      string         `json:"testcaseName"`
	TestcaseClassname string         `json:"testcaseClassname,omitempty"`
	TestcaseFile      string         `json:"testcaseFile,omitempty"`
	Testsuite         string         `json:"testsuite,omitempty"`
	Status            string         `json:"status"`
	Output            string         `json:"output,omitempty"`
	Baggage           map[string]any `json:"baggage,omitempty"`
}

type TestcasesRequest struct {
	Testcases []TestcaseRequest `json:"testcases"`
}

type TestResult struct {
	Package string
	Test    string
	Status  string
	Output  strings.Builder
}

func main() {
	cmd := &cli.Command{
		Name:  "greener-reporter",
		Usage: "Report Go test results to Greener",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     ingressEndpointFlag,
				Usage:    "Greener ingress endpoint",
				Sources:  cli.EnvVars("GREENER_INGRESS_ENDPOINT"),
				Required: true,
			},
			&cli.StringFlag{
				Name:     ingressAPIKeyFlag,
				Usage:    "Greener ingress API key",
				Sources:  cli.EnvVars("GREENER_INGRESS_API_KEY"),
				Required: true,
			},
		},
		Action: run,
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

type Reporter struct {
	endpoint  string
	apiKey    string
	sessionID string
	client    *http.Client
	results   map[string]*TestResult
}

func NewReporter(endpoint, apiKey string) *Reporter {
	return &Reporter{
		endpoint: strings.TrimSuffix(endpoint, "/"),
		apiKey:   apiKey,
		client:   &http.Client{},
		results:  make(map[string]*TestResult),
	}
}

func (r *Reporter) createSession() error {
	req := SessionRequest{
		Description: "Go test run",
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal session request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", r.endpoint+"/api/v1/ingress/sessions", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create session request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", r.apiKey)

	resp, err := r.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("send session request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create session failed: status=%d body=%s", resp.StatusCode, string(bodyBytes))
	}

	var sessionResp SessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sessionResp); err != nil {
		return fmt.Errorf("decode session response: %w", err)
	}

	r.sessionID = sessionResp.Id
	log.Printf("Created session: %s\n", r.sessionID)
	return nil
}

func (r *Reporter) submitResults() error {
	if len(r.results) == 0 {
		log.Println("No test results to submit")
		return nil
	}

	var testcases []TestcaseRequest
	for _, result := range r.results {
		testcases = append(testcases, TestcaseRequest{
			SessionId:         r.sessionID,
			TestcaseName:      result.Test,
			TestcaseClassname: result.Package,
			Testsuite:         result.Package,
			Status:            result.Status,
			Output:            result.Output.String(),
		})
	}

	req := TestcasesRequest{
		Testcases: testcases,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal testcases request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", r.endpoint+"/api/v1/ingress/testcases", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create testcases request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", r.apiKey)

	resp, err := r.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("send testcases request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("submit testcases failed: status=%d body=%s", resp.StatusCode, string(bodyBytes))
	}

	log.Printf("Submitted %d test results\n", len(testcases))
	return nil
}

func (r *Reporter) handleEvent(ev Event) {
	if ev.Test == "" {
		return
	}

	key := ev.Package + "/" + ev.Test

	switch ev.Action {
	case actionRun:
		r.results[key] = &TestResult{
			Package: ev.Package,
			Test:    ev.Test,
			Status:  statusError,
		}
	case actionPass:
		if result, ok := r.results[key]; ok {
			result.Status = statusPass
		} else {
			r.results[key] = &TestResult{
				Package: ev.Package,
				Test:    ev.Test,
				Status:  statusPass,
			}
		}
	case actionFail:
		if result, ok := r.results[key]; ok {
			result.Status = statusFail
		} else {
			r.results[key] = &TestResult{
				Package: ev.Package,
				Test:    ev.Test,
				Status:  statusFail,
			}
		}
	case actionSkip:
		if result, ok := r.results[key]; ok {
			result.Status = statusSkip
		} else {
			r.results[key] = &TestResult{
				Package: ev.Package,
				Test:    ev.Test,
				Status:  statusSkip,
			}
		}
	case actionOutput:
		if result, ok := r.results[key]; ok {
			result.Output.WriteString(ev.Output)
		}
	}
}

func run(ctx context.Context, c *cli.Command) error {
	endpoint := c.String(ingressEndpointFlag)
	apiKey := c.String(ingressAPIKeyFlag)

	reporter := NewReporter(endpoint, apiKey)

	if err := reporter.createSession(); err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	dec := json.NewDecoder(os.Stdin)
	for {
		var ev Event
		if err := dec.Decode(&ev); err != nil {
			if err.Error() == "EOF" {
				break
			}
			return fmt.Errorf("decode error: %w", err)
		}

		reporter.handleEvent(ev)
	}

	if err := reporter.submitResults(); err != nil {
		return fmt.Errorf("submit results: %w", err)
	}

	return nil
}
