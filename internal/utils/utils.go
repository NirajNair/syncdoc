package utils

import "fmt"

func GetWSAddr(httpAddr string) (string, error) {
	if len(httpAddr) > 8 && httpAddr[:8] == "https://" {
		return "wss://" + httpAddr[8:], nil
	}
	if len(httpAddr) > 7 && httpAddr[:7] == "http://" {
		return "ws://" + httpAddr[7:], nil
	}
	return "", fmt.Errorf("Error: address provided was not an HTTP(S) address")
}
