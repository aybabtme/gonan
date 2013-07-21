package gonan

import (
	"github.com/bmizerany/assert"
	"testing"
	"time"
)

type NopFlow struct{}

func (*NopFlow) Run(c *Context) error {
	return nil
}

func TestCreateRunWithNilRunArgsErrors(t *testing.T) {
	_, err := CreateRun(nil, new(Config))
	assert.T(t, err != nil)
	_, err = CreateRun(new(NopFlow), nil)
	assert.T(t, err != nil)
}

func TestCreateRunWithInvalidConfigErrors(t *testing.T) {
	f := new(NopFlow)
	c := new(Config)
	_, err := CreateRun(f, c)
	assert.T(t, err != nil)
}

type CountFlow struct {
	countChan  chan bool
	closeChan  chan bool
	iterations int64
}

func (f *CountFlow) Run(context *Context) error {
	f.countChan <- true
	return nil
}

func TestExecuteCompletedCorrectIterations(t *testing.T) {
	f := new(CountFlow)
	c := new(Config)
	f.countChan = make(chan bool)
	f.closeChan = make(chan bool)

	c.Iterations = 100
	c.Concurrency = 10
	go func() {
		for {
			select {
			case <-f.countChan:
				f.iterations++
			case <-f.closeChan:
				return
			}
		}
	}()
	r, err := CreateRun(f, c)
	assert.T(t, err == nil)
	res, err := r.Execute()
	f.closeChan <- true
	assert.T(t, err == nil)
	assert.Equal(t, f.iterations, c.Iterations)
	assert.T(t, res.EndTime.UnixNano() > res.StartTime.UnixNano())
}

type SleepyFlow struct{}

func (f *SleepyFlow) Run(context *Context) error {
	time.Sleep(250 * time.Millisecond)
	return nil
}

func TestCancelExecution(t *testing.T) {
	f := new(SleepyFlow)
	c := new(Config)
	c.Iterations = 100
	c.Concurrency = 10
	r, err := CreateRun(f, c)
	assert.T(t, err == nil)
	res, errchan := r.ExecuteAsync()
	time.Sleep(100 * time.Millisecond)
	r.Cancel()
	assert.T(t, <-errchan != nil)
	assert.T(t, res.NumSuccess < c.Iterations)
}

type SleepyHttpFlow struct{}

func (*SleepyHttpFlow) Run(context *Context) error {
	time.Sleep(250 * time.Millisecond)
	_, err := context.Client.Get("http://www.google.com")
	return err
}

func TestCancelWithHttpFlow(t *testing.T) {
	f := new(SleepyHttpFlow)
	c := new(Config)
	c.Iterations = 10
	c.Concurrency = 1
	r, err := CreateRun(f, c)
	assert.T(t, err == nil)
	res, errchan := r.ExecuteAsync()
	time.Sleep(100 * time.Millisecond)
	r.Cancel()
	assert.T(t, <-errchan != nil)
	assert.T(t, res.NumError > 0)
}

type SimpleGoogleHttpFlow struct{}

func (*SimpleGoogleHttpFlow) Run(c *Context) error {
	_, err := c.Client.Get("http://www.google.com")
	return err
}

func TestSimpleGoogleHttpFlow(t *testing.T) {
	f := new(SimpleGoogleHttpFlow)
	c := new(Config)
	c.Iterations = 1
	c.Concurrency = 1
	r, err := CreateRun(f, c)
	assert.T(t, err == nil)
	res, _ := r.Execute()
	assert.T(t, res.NumSuccess > 0)
	assert.T(t, res.NumHttpRequests > 0)
}
