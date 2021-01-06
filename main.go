package main

import (
    "context"
    "encoding/json"
    "fmt"
    "flag"
    "io"
    "io/ioutil"
    "log"
    "net/http"
    "os"
    "os/signal"
    "strings"
    "strconv"
    "syscall"
    "time"
)

const (
    ShutdownTimeout = 3 * time.Second
)

type ShutdownHook func()

type HandleResponse struct {
    Result  interface{} `json:"result"`
    Code    int         `json:"status_code"`
}

type Route struct {
    Path         []string       `json:"paths"`           // multiple url with same handler
    Method       []string       `json:"methods"`
    MockResult   HandleResponse `json:"mock_result"`
}

var (
    // command line arguments
    debug  = flag.Bool("debug",  false,       "debug mode")
    port   = flag.Int("port",    7001,        "agent server port")
    conf   = flag.String("conf", "apis.json", "json config file")
)

var (
    config map[string]interface{}
)

func main() {
    flag.Parse()

    if err := loadConfig(*conf); err != nil {
        log.Panicf("Failed to parse config file: %v reason: %v\n", *conf, err)
    }

    // you can add some shutdown hooks here
    startServer()
}

func startServer(hooks ...ShutdownHook) {
    log.Println("Start mock http server...")

    // init http server
    server := &http.Server{
        ReadTimeout:       10 * time.Second,
        WriteTimeout:      10 * time.Second,
        IdleTimeout:       30 * time.Second,
        ReadHeaderTimeout: 2 * time.Second,
        Addr:              fmt.Sprintf(":%v", *port),
        Handler:           http.HandlerFunc(DefaultHandler),
    }

    go func() {
        log.Printf("AgentRuntime start on %v\n", *port)
        if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("Failed to start http server\n%v", err)
        }
    }()

    // graceful shutdown
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit
    log.Println("Shutdown server ...")

    log.Println("Call shutdown hook ...")
    for _, hook := range hooks {
        hook()
    }

    ctx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
    defer cancel()
    if err := server.Shutdown(ctx); err != nil {
        log.Fatalf("shutdown error %v\n", err)
    }

    log.Println("exiting ...")
}

func loadConfig(filename string) error {
    if len(filename) <= 0 {
        return fmt.Errorf("empty config file name")
    }

    dataBytes, err := ioutil.ReadFile(filename)
    if err != nil {
        return err
    }

    config = make(map[string]interface{})
    err = json.Unmarshal(dataBytes, &config)
    if err != nil {
        return err
    }

    return nil
}

func PrettyPrintAsJson(obj interface{}) {
    b, err := json.MarshalIndent(obj, "", "  ")
    if err != nil {
        log.Panicln(err)
    }
    log.Println("\n" + string(b))
}

func ExistsInList(list []string, element string) bool {
    for _, el := range list {
        if el == element {
            return true
        }
    }

    return false
}

func writeResult(w http.ResponseWriter, r HandleResponse) int {
    w.WriteHeader(r.Code)
    json.NewEncoder(w).Encode(r)
    return r.Code
}

func IsValidRequest(r *http.Request, route *Route) bool {
    if !ExistsInList(route.Method, r.Method) {
        log.Printf("Illegal method: %v for %v, only allowed %v\n", r.Method, r.URL.Path, route.Method)
        return false
    }

    if r.Method != http.MethodGet && r.Method != http.MethodHead && !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
        // must be json request except GET or HEAD
        log.Printf("Not json request: %v\n", r.Header.Get("Content-Type"))
        return false
    }

    return true
}

func DefaultHandler(w http.ResponseWriter, r *http.Request) {
    start := time.Now()
    statusCode := http.StatusOK

    // force json
    w.Header().Set("Content-Type", "application/json; charset=UTF-8")

    url := r.URL.Path
    routes := config["apis"].([]interface{})
    found := false
    for _, routeDef := range routes {
        route := routeDef.(Route)
        for _, path := range route.Path {
            if strings.HasPrefix(url, path) {
                found = true
                statusCode = mock(w, r, &route)
            }
        }
    }

    if !found {
        log.Printf("not found handler for url: %v\n", url)
        statusCode = writeResult(w, HandleResponse{
            Result: fmt.Sprintf("Unsupported URL %v", url),
            Code:    http.StatusBadRequest,
        })
    }

    // access oklog
    log.Printf("ACCESS method:[%v] url:[%v] client_ip:[%v] user_agent:[%v] referer:[%v] response_time:[%v] status_code:[%v]\n",
        r.Method, r.URL.String(), r.RemoteAddr, r.UserAgent(), r.Referer(), time.Since(start).String(), statusCode)
}

func mock(w http.ResponseWriter, r *http.Request, route *Route) int {
    if !IsValidRequest(r, route) {
        return writeResult(w, HandleResponse{
            Result:  fmt.Sprintf("Illegal request: %v %v", r.Method, r.URL.Path),
            Code:    http.StatusBadRequest,
        })
    }

    log.Printf("MOCK API: %v %v\n", r.Method, r.URL)

    result := HandleResponse{
        Result:  fmt.Sprintf("request %v received", r.URL),
        Code:    http.StatusOK,
    }

    //To allocate slice for request body
    length, err := strconv.Atoi(r.Header.Get("Content-Length"))
    if err != nil {
        log.Printf("error on get content length from request: %v\n", err)
        if r.Method != http.MethodGet && r.Method != http.MethodHead {
            result = HandleResponse{
                Result:  "failed to get request length",
                Code:    http.StatusInternalServerError,
            }
            return writeResult(w, result)
        }
    }

    //Read body data to parse json
    var data interface{}

    if length > 0 {
        body := make([]byte, length)
        length, err = r.Body.Read(body)
        if err != nil && err != io.EOF {
            result = HandleResponse{
                Result:  "failed to read request body",
                Code:    http.StatusInternalServerError,
            }
            return writeResult(w, result)
        }

        log.Printf("%v %v request body:%v\n", r.Method, r.URL, string(body))

        err = json.Unmarshal(body[:length], &data)
        if err != nil {
            result = HandleResponse{
                Result:  "failed to parse request body to json object",
                Code:    http.StatusInternalServerError,
            }
            return writeResult(w, result)
        }

        PrettyPrintAsJson(data)
    }

    ret := struct {
        Ack    string      `json:"ack"`
        Url    string      `json:"url"`
        Method string      `json:"method"`
        Body   interface{} `json:"body"`
        Result interface{} `json:"result"`
    } {
        Ack:    "OK",
        Url:    r.URL.String(),
        Method: r.Method,
        Body:   data,
        Result: route.MockResult,
    }

    result = HandleResponse{
        Result:  ret,
        Code:    http.StatusOK,
    }

    return writeResult(w, result)
}
