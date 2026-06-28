package common

import (
    "bytes"
    "crypto/tls"
    "encoding/json"
    "fmt"
    "net/http"
    "sync"
    "time"

    "roproxy/internal/config"
)

type APIRequest struct {
    Endpoint  string
    Payload   map[string]interface{}
    Retries   uint32 
}

// UnboundedAPIQueue is a thread-safe unbounded queue that grows until RAM limit.
// Used for API consumer to never lose requests even during API downtime.
type UnboundedAPIQueue struct {
    items []APIRequest
    mutex sync.Mutex
    cond  *sync.Cond
}

func NewUnboundedAPIQueue() *UnboundedAPIQueue {
    q := &UnboundedAPIQueue{
        items: make([]APIRequest, 0, 1000),
    }
    q.cond = sync.NewCond(&q.mutex)
    return q
}

func (q *UnboundedAPIQueue) Push(item APIRequest) {
    q.mutex.Lock()
    q.items = append(q.items, item)
    q.cond.Signal()
    q.mutex.Unlock()
}

func (q *UnboundedAPIQueue) Pop() APIRequest {
    q.mutex.Lock()
    defer q.mutex.Unlock()
    
    for len(q.items) == 0 {
        q.cond.Wait()
    }
    
    item := q.items[0]
    q.items = q.items[1:]
    return item
}

func (q *UnboundedAPIQueue) Size() int {
    q.mutex.Lock()
    defer q.mutex.Unlock()
    return len(q.items)
}

type APIConsumer struct {
    queue      *UnboundedAPIQueue
    baseURL    string
    apiKey     string
    httpClient *http.Client
}

var globalAPIConsumer *APIConsumer

func GetAPIConsumer() *APIConsumer {
    return globalAPIConsumer
}

func (ac *APIConsumer) QueueSize() int {
    if ac == nil {
        return 0
    }
    return ac.queue.Size()
}

func InitAPIConsumer(cfg *config.Config) {

    if cfg.API == nil || cfg.API.URL == "" || cfg.API.Key == "" {
        Log(LogAPI, LogInfo, "API configuration failed")
    }

    baseURL := cfg.API.URL
    apiKey := cfg.API.Key
    
    if baseURL == "" || apiKey == "" {
        Log(LogAPI, LogInfo, "InitAPIConsumer skipped - no config")
        return
    }

    Log(LogAPI, LogInfo, "InitAPIConsumer starting - URL=%s", baseURL)

    globalAPIConsumer = &APIConsumer{
        queue:   NewUnboundedAPIQueue(),
        baseURL: baseURL,
        apiKey:  apiKey,
        httpClient: &http.Client{
            Timeout: 10 * time.Second,
            Transport: &http.Transport{
                TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
            },
        },
    }

    go globalAPIConsumer.consumeLoop()
    Log(LogAPI, LogInfo, "Consumer goroutine started")
}

// Don't use directly, was used by the wrapper `SendToAPI`
func SendToAPIInternal(endpoint string, payload map[string]interface{}) {
    if globalAPIConsumer == nil {
        Log(LogAPI, LogError, "SendToAPI called but globalAPIConsumer is nil (endpoint=%s)", endpoint)
        return
    }

    Log(LogAPI, LogVerbose, "SendToAPI: %s (queue size before: %d)", endpoint, globalAPIConsumer.queue.Size())

    globalAPIConsumer.queue.Push(APIRequest{
        Endpoint:  endpoint,
        Payload:   payload,
    })
    
    // Log warning if queue is getting large
    size := globalAPIConsumer.queue.Size()
    if size > 0 && size%100000 == 0 {
        Log(LogAPI, LogWarning, "API queue size reached %d items (API may be down)", size)
    }
}

func (ac *APIConsumer) consumeLoop() {
    Log(LogAPI, LogVerbose, "consumeLoop started, waiting for requests...")
    for {
        req := ac.queue.Pop()
        Log(LogAPI, LogVerbose, "Processing request: %s (queue size: %d)", req.Endpoint, ac.queue.Size())
        
        if req.Retries >= 100 {
            Log(LogAPI, LogError, "Imposible to delivery request %s", req)
        } else {
            ac.sendRequest(req)
        }
    }
}

func (ac *APIConsumer) sendRequest(req APIRequest) {
    url := fmt.Sprintf("%s/%s", ac.baseURL, req.Endpoint)

    jsonData, err := json.Marshal(req.Payload)
    if err != nil {
        Log(LogAPI, LogError, "Failed to marshal request: %v", err)
        return
    }

    for {
        httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
        if err != nil {
            Log(LogAPI, LogError, "Failed to create request: %v", err)
            time.Sleep(1 * time.Second)
            continue
        }

        httpReq.Header.Set("Content-Type", "application/json")
        httpReq.Header.Set("X-API-KEY", ac.apiKey)

        resp, err := ac.httpClient.Do(httpReq)
        if err != nil {
            Log(LogAPI, LogVerbose, "Request failed (will retry): %v", err)
            time.Sleep(1 * time.Second)
            continue
        }

        resp.Body.Close()

        if resp.StatusCode >= 200 && resp.StatusCode < 300 {
            Log(LogAPI, LogVerbose, "Request sent: %s", req.Endpoint)
            return
        }

        Log(LogAPI, LogVerbose, "[%d] Request failed with status %d (will retry) [%s]", ac.queue.Size(), resp.StatusCode, jsonData)
        req.Retries++
        ac.queue.Push(req)
        time.Sleep(1 * time.Second)
        return
    }
}
