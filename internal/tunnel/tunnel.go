package tunnel

import (
	"context"
	"fmt"

	"github.com/NirajNair/syncdoc/internal/config"
	"golang.ngrok.com/ngrok/v2"
)

type AgentFactory interface {
	NewAgent(token string) (Agent, error)
}

type Agent interface {
	ForwardHTTP(ctx context.Context, addr string) (Tunnel, error)
}

type ngrokTunnel struct {
	forwarder ngrok.EndpointForwarder
}

func (t *ngrokTunnel) URL() string {
	if t.forwarder.URL() == nil {
		return ""
	}
	return t.forwarder.URL().String()
}

func (t *ngrokTunnel) Close() error {
	return t.forwarder.Close()
}

type defaultAgentFactory struct{}

func (f *defaultAgentFactory) NewAgent(token string) (Agent, error) {
	agent, err := ngrok.NewAgent(ngrok.WithAuthtoken(token))
	if err != nil {
		return nil, err
	}
	return &defaultAgent{agent: agent}, nil
}

type defaultAgent struct {
	agent ngrok.Agent
}

func (a *defaultAgent) ForwardHTTP(ctx context.Context, addr string) (Tunnel, error) {
	forwarder, err := a.agent.Forward(ctx, ngrok.WithUpstream(addr))
	if err != nil {
		return nil, err
	}
	return &ngrokTunnel{forwarder: forwarder}, nil
}

var agentFactory AgentFactory = &defaultAgentFactory{}

func setAgentFactory(factory AgentFactory) {
	agentFactory = factory
}

func getNgrokToken() (string, error) {
	token, err := config.GetNgrokToken()
	if err != nil {
		return "", fmt.Errorf("Could not get Ngrok token: %w", err)
	}
	if token == "" {
		return "", fmt.Errorf("Ngrok token not set. Run: syncdoc config set-ngrok-token <token>")
	}
	return token, nil
}

func StartHTTPTunnel(ctx context.Context, addr string) (Tunnel, error) {
	token, err := getNgrokToken()
	if err != nil {
		return nil, err
	}

	agent, err := agentFactory.NewAgent(token)
	if err != nil {
		return nil, fmt.Errorf("Error creating ngrok agent: %v", err.Error())
	}

	tunnel, err := agent.ForwardHTTP(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("Error creating ngrok forwarder: %v", err.Error())
	}

	return tunnel, nil
}
