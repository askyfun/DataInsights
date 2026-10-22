package datasource

import (
	"context"
	"errors"
	"strings"
	"testing"

	"data-insights/internal/datasource"
	"data-insights/internal/model"
)

// capsStubConnection embeds a nil Connection so only the methods this file
// needs are reachable (same loud-failure pattern as the handler mocks).
type capsStubConnection struct {
	datasource.Connection
	caps   *datasource.DialectCapabilities
	err    error
	closed bool
}

func (c *capsStubConnection) Close() error { c.closed = true; return nil }

func (c *capsStubConnection) Capabilities(context.Context) (*datasource.DialectCapabilities, error) {
	return c.caps, c.err
}

func TestGetCapabilitiesProjectsDialectAndFlags(t *testing.T) {
	conn := &capsStubConnection{caps: &datasource.DialectCapabilities{
		SupportsGroupingSets:    true,
		SupportsPercentileCont:  true,
		SupportsWindowFunctions: true,
		PercentileStrategy:      "percentile_cont",
	}}
	s := &datasourceService{
		connectFn: func(context.Context, *model.Datasource) (datasource.Connection, error) {
			return conn, nil
		},
		getDatasourceModelFn: func(context.Context, int) (*model.Datasource, error) {
			return &model.Datasource{ID: 3, Type: "postgresql"}, nil
		},
	}

	got, err := s.GetCapabilities(context.Background(), 3)
	if err != nil {
		t.Fatalf("GetCapabilities: %v", err)
	}
	if got.Dialect != "postgresql" {
		t.Errorf("dialect = %q, want postgresql", got.Dialect)
	}
	if !got.SupportsGroupingSets || !got.SupportsPercentileCont || !got.SupportsWindowFunctions {
		t.Errorf("boolean flags not projected: %+v", got)
	}
	if got.PercentileStrategy != "percentile_cont" {
		t.Errorf("percentile strategy = %q", got.PercentileStrategy)
	}
	if !conn.closed {
		t.Error("connection must be closed after probing")
	}
}

func TestGetCapabilitiesPropagatesProbeError(t *testing.T) {
	s := &datasourceService{
		connectFn: func(context.Context, *model.Datasource) (datasource.Connection, error) {
			return &capsStubConnection{err: errors.New("dialect probe failed")}, nil
		},
		getDatasourceModelFn: func(context.Context, int) (*model.Datasource, error) {
			return &model.Datasource{ID: 4, Type: "mysql"}, nil
		},
	}

	_, err := s.GetCapabilities(context.Background(), 4)
	if err == nil || !strings.Contains(err.Error(), "failed to probe capabilities") {
		t.Fatalf("expected wrapped probe error, got %v", err)
	}
}

func TestGetCapabilitiesRejectsNilProbe(t *testing.T) {
	s := &datasourceService{
		connectFn: func(context.Context, *model.Datasource) (datasource.Connection, error) {
			return &capsStubConnection{}, nil
		},
		getDatasourceModelFn: func(context.Context, int) (*model.Datasource, error) {
			return &model.Datasource{ID: 5, Type: "clickhouse"}, nil
		},
	}

	_, err := s.GetCapabilities(context.Background(), 5)
	if err == nil || !strings.Contains(err.Error(), "capabilities unavailable for dialect clickhouse") {
		t.Fatalf("expected nil-probe error, got %v", err)
	}
}
