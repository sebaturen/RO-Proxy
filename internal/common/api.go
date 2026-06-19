package common

import (
    "bytes"
    "crypto/tls"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "time"
)

type APIRequest struct {
    Endpoint  string
    Payload   map[string]interface{}
}

type APIConsumer struct {
    queue      chan APIRequest
    baseURL    string
    apiKey     string
    httpClient *http.Client
    verbose    bool
}

var globalAPIConsumer *APIConsumer

func GetAPIConsumer() *APIConsumer {
	return globalAPIConsumer
}

func (ac *APIConsumer) QueueSize() int {
	if ac == nil {
		return 0
	}
	return len(ac.queue)
}

func InitAPIConsumer(baseURL, apiKey string, verbose bool) {
    if baseURL == "" || apiKey == "" {
        return
    }

    globalAPIConsumer = &APIConsumer{
        queue:   make(chan APIRequest, 10000),
        baseURL: baseURL,
        apiKey:  apiKey,
        httpClient: &http.Client{
            Timeout: 10 * time.Second,
            Transport: &http.Transport{
                TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
            },
        },
        verbose: verbose,
    }

    go globalAPIConsumer.consumeLoop()
}

func SendToAPI(endpoint string, payload map[string]interface{}) {
    if globalAPIConsumer == nil {
        return
    }

    globalAPIConsumer.queue <- APIRequest{
        Endpoint:  endpoint,
        Payload:   payload,
    }
}

func (ac *APIConsumer) consumeLoop() {
    for req := range ac.queue {
        ac.sendRequest(req)
    }
}

func (ac *APIConsumer) sendRequest(req APIRequest) {
    url := fmt.Sprintf("%s/%s", ac.baseURL, req.Endpoint)

    jsonData, err := json.Marshal(req.Payload)
    if err != nil {
        log.Printf("Failed to marshal API request: %v", err)
        return
    }

    for {
        httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
        if err != nil {
            log.Printf("Failed to create API request: %v", err)
            time.Sleep(1 * time.Second)
            continue
        }

        httpReq.Header.Set("Content-Type", "application/json")
        httpReq.Header.Set("X-API-KEY", ac.apiKey)

        resp, err := ac.httpClient.Do(httpReq)
        if err != nil {
            if ac.verbose {
                log.Printf("API request failed (will retry): %v", err)
            }
            time.Sleep(1 * time.Second)
            continue
        }

        resp.Body.Close()

        if resp.StatusCode >= 200 && resp.StatusCode < 300 {
            if ac.verbose {
                log.Printf("API request sent: %s", req.Endpoint)
            }
            return
        }

        if ac.verbose {
            log.Printf("API request failed with status %d (will retry)", resp.StatusCode)
        }
        time.Sleep(1 * time.Second)
    }
}
