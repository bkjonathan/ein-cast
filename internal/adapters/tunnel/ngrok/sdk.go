package ngrok

import (
	"context"
	"errors"
	"io"
	"log/slog"

	sdk "golang.ngrok.com/ngrok/v2"
)

// connector opens a tunnel to ngrok. It is the one place that touches the SDK,
// so everything else is tested without a network or an account.
type connector interface {
	// Forward signs in with authtoken and publishes an endpoint that forwards to
	// upstream (an http URL). onEvent reports connection changes afterwards.
	Forward(ctx context.Context, authtoken, upstream string, onEvent func(event)) (endpoint, error)
}

// endpoint is a published ngrok endpoint.
type endpoint interface {
	// URL is the public address.
	URL() string
	// Done is closed when the endpoint has ended, whoever ended it.
	Done() <-chan struct{}
	// Close ends the endpoint and the connection to ngrok. It is safe to repeat.
	Close(ctx context.Context) error
}

// event is a change in the connection to ngrok.
type event struct {
	connected bool
	err       error // why it disconnected, if known
}

// sdkConnector is the real connector.
type sdkConnector struct{}

func (sdkConnector) Forward(ctx context.Context, authtoken, upstream string, onEvent func(event)) (endpoint, error) {
	agent, err := sdk.NewAgent(
		sdk.WithAuthtoken(authtoken),
		// The SDK's own logging is switched off: what matters reaches us as
		// errors and events, and it is not ours to vouch that it never prints a
		// credential.
		sdk.WithLogger(slog.New(slog.DiscardHandler)),
		sdk.WithEventHandler(func(e sdk.Event) {
			switch ev := e.(type) {
			case *sdk.EventAgentConnectSucceeded:
				onEvent(event{connected: true})
			case *sdk.EventAgentDisconnected:
				onEvent(event{err: ev.Error})
			}
		}),
	)
	if err != nil {
		return nil, err
	}
	if err := agent.Connect(ctx); err != nil {
		return nil, err
	}
	fwd, err := agent.Forward(ctx, sdk.WithUpstream(upstream))
	if err != nil {
		_ = agent.Disconnect()
		return nil, err
	}
	return &sdkEndpoint{agent: agent, fwd: fwd}, nil
}

type sdkEndpoint struct {
	agent sdk.Agent
	fwd   sdk.EndpointForwarder
}

func (e *sdkEndpoint) URL() string           { return e.fwd.URL().String() }
func (e *sdkEndpoint) Done() <-chan struct{} { return e.fwd.Done() }

func (e *sdkEndpoint) Close(ctx context.Context) error {
	err := e.fwd.CloseWithContext(ctx)
	// Disconnecting ends the session too, so nothing keeps the token's connection open.
	if derr := e.agent.Disconnect(); err == nil && !errors.Is(derr, io.EOF) {
		err = derr
	}
	return err
}
