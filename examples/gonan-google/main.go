package main

import (
	"fmt"
	"github.com/csfrancis/gonan"
	"sync/atomic"
)

type GoogleFlow struct {
	Count int64
}

func (f *GoogleFlow) Run(c *gonan.Context) error {
	count := atomic.AddInt64(&f.Count, 1)
	r, err := c.Client.Get(fmt.Sprintf("http://www.google.com/?%d", count))
	if err != nil {
		fmt.Printf("%v\n", err)
	}
	r.Body.Close()
	return err
}

func main() {
	f := new(GoogleFlow)
	c := new(gonan.Config)
	c.Iterations = 5000
	c.Concurrency = 500
	r, _ := gonan.CreateRun(f, c)
	res, _ := r.Execute()
	fmt.Printf("Executed run in %v\n", res.EndTime.Sub(res.StartTime))
	fmt.Printf("%d requests sent (%d success, %d errors).\n", res.NumHttpRequests, res.NumSuccess, res.NumError)
}
