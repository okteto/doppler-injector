package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	corev1 "k8s.io/api/core/v1"
)

type dopplerResponse struct {
	Success  bool              `json:"success"`
	Messages []string          `json:"messages"`
	Keys     map[string]string `json:"keys"`
}

func (whsvr *WebhookServer) getEnvFromDoppler(pipeline, environment string) ([]corev1.EnvVar, error) {
	url := fmt.Sprintf("https://api.doppler.com/environments/%s/fetch_keys", environment)

	req, err := http.NewRequest("POST", url, nil)
	req.Header.Set("api-key", whsvr.apiKey)
	req.Header.Set("pipeline", pipeline)
	req.Header.Set("client-sdk", "api")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var doppler dopplerResponse
	if err := json.Unmarshal(body, &doppler); err != nil {
		return nil, err
	}

	if !doppler.Success {
		return nil, fmt.Errorf("%v", doppler.Messages)
	}

	env := make([]corev1.EnvVar, 0)

	for k, v := range doppler.Keys {
		e := corev1.EnvVar{
			Name:  k,
			Value: v,
		}

		env = append(env, e)
	}

	return env, nil
}
