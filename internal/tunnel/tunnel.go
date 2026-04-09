package tunnel

import (
	"context"
	"fmt"

	"golang.ngrok.com/ngrok/v2"
)

const TOKEN = "3C7iNOiAPdB8rsxC5SZgaDO0tIf_7AXLx1wxoQndC87eb5J7Q"

func Run(ctx context.Context, addr string) (ngrok.EndpointForwarder, error) {
	agent, err := ngrok.NewAgent(ngrok.WithAuthtoken(TOKEN))
	if err != nil {
		return nil, fmt.Errorf("Error creating ngrok agent: %v", err.Error())
	}

	tunnel, err := agent.Forward(ctx,
		ngrok.WithUpstream(addr),
	)

	if err != nil {
		return nil, fmt.Errorf("Error creating ngrok forwarder: %v", err.Error())
	}

	fmt.Println("Endpoint online: forwarding from", tunnel.URL(), "to", addr)

	return tunnel, nil
}
