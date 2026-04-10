package tunnel

import (
	"context"
	"fmt"

	"github.com/NirajNair/syncdoc/internal/config"
	"golang.ngrok.com/ngrok/v2"
)

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

func StartHTTPTunnel(ctx context.Context, addr string) (ngrok.EndpointForwarder, error) {
	token, err := getNgrokToken()
	if err != nil {
		return nil, err
	}

	agent, err := ngrok.NewAgent(ngrok.WithAuthtoken(token))
	if err != nil {
		return nil, fmt.Errorf("Error creating ngrok agent: %v", err.Error())
	}

	tunnel, err := agent.Forward(ctx,
		ngrok.WithUpstream(addr),
	)

	if err != nil {
		return nil, fmt.Errorf("Error creating ngrok forwarder: %v", err.Error())
	}

	return tunnel, nil
}
