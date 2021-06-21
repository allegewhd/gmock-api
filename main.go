package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	ShutdownTimeout = 3 * time.Second
)

type ShutdownHook func()

type MockResponse struct {
    Type         string         `json:"content_type"`
	Data         interface{}    `json:"data"`
	Code         int            `json:"status_code"`
}

type Route struct {
	Path         []string       `json:"path"`
	Method       []string       `json:"method"`
	Accept       []string       `json:"accept"`
	Response     MockResponse   `json:"response"`
}

type Config struct {
	Settings map[string]interface{} `json:"settings"`
	APIs     []Route                `json:"apis"`
}

var (
	// command line arguments
	debug = flag.Bool("debug", false, "debug mode")
	port  = flag.Int("port", 7001, "agent server port")
	conf  = flag.String("conf", "apis.json", "json config file")
)

var (
	config *Config
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
		Handler:           http.HandlerFunc(defaultHandler),
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

	config = &Config{}
	err = json.Unmarshal(dataBytes, config)
	if err != nil {
		return err
	}

	return nil
}

func printAsJson(obj interface{}) {
	b, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		log.Panicln(err)
	}
	log.Println("\n" + string(b))
}

func contains(list []string, element string) bool {
	for _, el := range list {
		if el == element {
			return true
		}
	}

	return false
}

func isAcceptableReqType(list []string, element string) bool {
	for _, el := range list {
		if el == "all" {
			return true
		}
		if strings.Contains(element, el) {
			return true
		}
	}

	return false
}

func writeResult(w http.ResponseWriter, r MockResponse) int {
	w.Header().Set("Content-Type", r.Type)
	w.WriteHeader(r.Code)

	ret := r.Data

	if *debug {
		log.Println("write response body:")
		printAsJson(ret)
	}

	if strings.Contains(r.Type, "application/json") {
		wrap := struct {
			Message string `json:"message"`
		}{}
		switch v := ret.(type) {
		case bool, float64, int, string:
			wrap.Message = fmt.Sprintf("%v", v)
			_ = json.NewEncoder(w).Encode(wrap)
		default:
			_ = json.NewEncoder(w).Encode(ret)
		}
	} else {
		// default to text/plain
		_, err := w.Write([]byte(fmt.Sprintf("%s", ret)))
		if err != nil {
			log.Printf("Error on write %v to response stream.\n", ret)
		}
	}
	return r.Code
}

func checkRequest(r *http.Request, route *Route) bool {
	strictMode := config.Settings["strict_mode"].(bool)

	if !strictMode {
		return true
	}

	if !contains(route.Method, r.Method) {
		log.Printf("Illegal method: %v for %v, only allowed %v\n", r.Method, r.URL.Path, route.Method)
		return false
	}

	reqType := r.Header.Get("Content-Type")
	if !isAcceptableReqType(route.Accept, reqType) {
		log.Printf("Not acceptable request type. expected: [%v] actual: [%v]\n", route.Accept, reqType)
		return false
	}

	return true
}

func defaultHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	statusCode := http.StatusOK
	defaultContentType := config.Settings["default_content_type"].(string)

	url := r.URL.Path
	found := false
	for _, route := range config.APIs {
		for _, path := range route.Path {
			if url == path {
				found = true
				statusCode = mock(w, r, &route)
				break
			}
		}

		if found {
			break
		}

		// try to find prefix match definition
		for _, path := range route.Path {
			if strings.HasPrefix(url, path) {
				found = true
				statusCode = mock(w, r, &route)
				break
			}
		}
	}

	if !found {
		log.Printf("not found handler for url: %v\n", url)
		statusCode = writeResult(w, MockResponse{
			Type:   defaultContentType,
			Data:   fmt.Sprintf("Unsupported URL %v", url),
			Code:   http.StatusBadRequest,
		})
	}

	// access log
	log.Printf("ACCESS method:[%v] url:[%v] client_ip:[%v] user_agent:[%v] referer:[%v] response_time:[%v] status_code:[%v]\n",
		r.Method, r.URL.String(), r.RemoteAddr, r.UserAgent(), r.Referer(), time.Since(start).String(), statusCode)
}

func mock(w http.ResponseWriter, r *http.Request, route *Route) int {
	if !checkRequest(r, route) {
		return writeResult(w, MockResponse{
			Type:   route.Response.Type,
			Data:   fmt.Sprintf("Illegal request: %v %v", r.Method, r.URL.Path),
			Code:   http.StatusBadRequest,
		})
	}

	log.Printf("MOCK API: %v %v\n", r.Method, r.URL)

	strictMode := config.Settings["strict_mode"].(bool)

	if strictMode && len(r.Header.Get("Content-Length")) > 0 {
		// parse request body
		size, err := strconv.Atoi(r.Header.Get("Content-Length"))
		if err != nil {
			log.Printf("error on get content length from request: %v\n", err)
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				return writeResult(w, MockResponse{
					Type:   route.Response.Type,
					Data:   "failed to get request length",
					Code:   http.StatusInternalServerError,
				})
			}
		}

		// read body data to parse json
		if size > 0 {
			body := make([]byte, size)
			size, err = r.Body.Read(body)
			if err != nil && err != io.EOF {
				return writeResult(w, MockResponse{
					Type:   route.Response.Type,
					Data: "failed to read request body",
					Code:   http.StatusInternalServerError,
				})
			}

			log.Printf("%v %v request body:%v\n", r.Method, r.URL, string(body))

			var data interface{}
			err = json.Unmarshal(body[:size], &data)
			if err != nil {
				return writeResult(w, MockResponse{
					Type:   route.Response.Type,
					Data: "failed to parse request body to json object",
					Code:   http.StatusInternalServerError,
				})
			}

			if *debug {
				log.Println("received:")
				printAsJson(data)
			}
		}
	}

	return writeResult(w, route.Response)
}
