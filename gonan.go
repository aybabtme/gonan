package gonan

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

type Config struct {
	Iterations  int64
	Concurrency int
	Domain      string
	GatewayId   string
	Server      string
	MaxRetries  int
	ReadTimeout int
}

type Result struct {
	Name            string
	StartTime       time.Time
	EndTime         time.Time
	NumSuccess      int64
	NumError        int64
	RequestRetries  int64
	NumHttpRequests int64
	Config
}

type Context struct {
	Client *http.Client
	Config
}

type Flow interface {
	Run(*Context) error
}

type Run interface {
	ExecuteAsync() (*Result, chan error)
	Execute() (*Result, error)
	Cancel()
}

type gonanRun struct {
	Flow
	Config
	Name       string
	CancelChan chan chan bool
	Canceled   bool
	ClientCache chan *http.Client
}

type gonanError struct {
	msg    string
	source string
}

type gonanRoundTripper struct {
	Transport   *http.Transport
	Canceled    *bool
	FlowName    *string
	NumRequests int
}

func (rt *gonanRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if *rt.Canceled {
		return nil, createError(errCanceled)
	}
	req.Header["User-Agent"] = []string{fmt.Sprintf("Godzilla/1.0 (gonan; flow=%s)", *rt.FlowName)}
	rt.NumRequests++
	return rt.Transport.RoundTrip(req)
}

func (rt *gonanRoundTripper) Reset() {
	rt.NumRequests = 0
}

const errInvalidArgs = "invalid arguments"
const errInvalidConfig = "invalid configuration"
const errCanceled = "canceled"

func (e *gonanError) Error() string {
	return fmt.Sprintf("%s, %s", e.source, e.msg)
}

func createError(msg string) error {
	if _, file, lineno, ok := runtime.Caller(1); ok {
		return &gonanError{msg: msg, source: fmt.Sprintf("%v:%v", file, lineno)}
	}
	return &gonanError{msg, ""}
}

func validateConfig(config *Config) error {
	if config.Iterations == 0 {
		return createError(errInvalidConfig)
	}
	if config.Concurrency == 0 {
		return createError(errInvalidConfig)
	}
	return nil
}

func (r *gonanRun) processFlow(result *Result) error {
	context := new(Context)
	client := r.getClient()
	defer r.releaseClient(client)
	context.Client = client
	context.Config = r.Config
	err := r.Flow.Run(context)
	atomic.AddInt64(&result.NumHttpRequests, int64(client.Transport.(*gonanRoundTripper).NumRequests))
	return err
}

func (r *gonanRun) getClient() *http.Client {
	return <- r.ClientCache
}

func (r *gonanRun) releaseClient(client *http.Client) {
	client.Transport.(*gonanRoundTripper).Reset()
	r.ClientCache <- client 
}

func (r *gonanRun) initClientCache() {
	r.ClientCache = make(chan *http.Client, r.Config.Concurrency)
	for i := 0; i < r.Config.Concurrency; i++ {
		rt := &gonanRoundTripper{&http.Transport{Proxy: http.ProxyFromEnvironment, DisableKeepAlives: false}, &r.Canceled, &r.Name, 0}
		rt.Transport.ResponseHeaderTimeout = time.Duration(r.Config.ReadTimeout) * time.Second
		jar, _ := cookiejar.New(nil)
		client := &http.Client{rt, nil, jar}
		r.ClientCache <- client
	}
}

func (r *gonanRun) ExecuteAsync() (*Result, chan error) {
	result := new(Result)
	result.Name = r.Name
	result.Config = r.Config

	var wg sync.WaitGroup
	var sem = make(chan int, r.Config.Concurrency)
	errchan := make(chan error)
	result.StartTime = time.Now()

	r.initClientCache()
	go func() {
		var cancelchan chan bool
		var err error
		for i := int64(0); i < r.Config.Iterations && !r.Canceled; i++ {
			select {
			case sem <- 1:
				wg.Add(1)
				go func() {
					defer func() {
						wg.Done()
						<-sem
					}()
					flowErr := r.processFlow(result)
					if flowErr == nil {
						atomic.AddInt64(&result.NumSuccess, 1)
					} else {
						atomic.AddInt64(&result.NumError, 1)
					}
				}()
			case cancelchan = <-r.CancelChan:
				err = createError(errCanceled)
				r.Canceled = true
				break
			}
		}
		wg.Wait()
		result.EndTime = time.Now()
		if cancelchan != nil {
			cancelchan <- true
		}
		errchan <- err
	}()
	return result, errchan
}

func (r *gonanRun) Execute() (*Result, error) {
	res, errchan := r.ExecuteAsync()
	return res, <-errchan
}

func (r *gonanRun) Cancel() {
	c := make(chan bool)
	r.CancelChan <- c
	<-c
}

func CreateRun(flow Flow, config *Config) (Run, error) {
	var err error = nil
	if flow == nil || config == nil {
		return nil, createError(errInvalidConfig)
	}
	if err = validateConfig(config); err != nil {
		return nil, err
	}
	run := &gonanRun{flow, *config,
		reflect.TypeOf(flow).Elem().Name(), make(chan chan bool), false, nil}
	return run, nil
}
