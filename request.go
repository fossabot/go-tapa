package tapa

import (
	"fmt"
	"math/rand"
	"net/http"
	"reflect"
	"time"

	"github.com/johnmcdnl/go-tapa/stopwatch"
	pb "gopkg.in/cheggaaa/pb.v1"
)

// ExpecationFunc validates the response is valid
type ExpecationFunc func(resp *http.Response) error

// Request contains details of the request and response
type Request struct {
	name            string
	client          *http.Client
	request         *http.Request
	jobs            chan *http.Request
	delayMin        time.Duration
	delayMax        time.Duration
	expectations    []ExpecationFunc
	users           int
	requestsPerUser int
	durations       []time.Duration
	errors          []error
	rand            *rand.Rand
	progressBar     *pb.ProgressBar
}

// NewRequest creates a new request
func NewRequest(req *http.Request) *Request {
	request := &Request{
		request: req,
		rand:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	return request
}

// NewRequestMust creates a new request or panics
func NewRequestMust(req *http.Request, err error) *Request {
	if err != nil {
		panic(err)
	}
	return NewRequest(req)
}

// Name returns the name of the test
func (r *Request) Name() string {
	if r.name == "" {
		r.name = fmt.Sprintf("%s %s", r.request.Method, r.request.URL.String())
	}
	return r.name
}

// WithName allows user to set a name for a test request
func (r *Request) WithName(name string) {
	r.name = name
}

// WithUsers defines how many concurrent users there will be
func (r *Request) WithUsers(users int) {
	r.users = users
}

// WithRequestsPerUser gives a default number of requests per user
func (r *Request) WithRequestsPerUser(reqsPerUser int) {
	r.requestsPerUser = reqsPerUser
}

// WithExpectation adds an ExpecationFunc to the request
func (r *Request) WithExpectation(fn ExpecationFunc) {
	r.expectations = append(r.expectations, fn)
}

// WithClient defines what client will be used
func (r *Request) WithClient(client *http.Client) {
	r.client = client
}

// WithDelay adds a randomised delay between requests
func (r *Request) WithDelay(min, max time.Duration) {
	if min > max {
		r.delayMin = max
		r.delayMax = min
		return
	}
	r.delayMin = min
	r.delayMax = max

}

func (r *Request) getDelay() time.Duration {
	if r.delayMin == r.delayMin {
		return r.delayMin
	}

	return time.Duration(r.rand.Intn(int(r.delayMax-r.delayMin)) + int(r.delayMin))
}

func (r *Request) execute() {
	r.progressBar.Start()

	defer r.progressBar.Finish()
	size := r.users * r.requestsPerUser

	r.jobs = make(chan *http.Request, r.users)
	responses := make(chan interface{}, size)

	for worker := 1; worker <= r.users; worker++ {
		go r.executeRequestJobs(responses)
	}

	for j := 1; j <= size; j++ {
		req := *r.request
		r.jobs <- &req
	}
	close(r.jobs)

	for a := 1; a <= size; a++ {
		response := <-responses
		switch x := response.(type) {
		default:
			panic(fmt.Sprintf("unhandled response type %v", reflect.TypeOf(x)))
		case time.Duration:
			r.durations = append(r.durations, x)
		case error:
			r.errors = append(r.errors, x)
		}

	}

}

func (r *Request) executeRequestJobs(responses chan interface{}) {

	for req := range r.jobs {
		time.Sleep(r.getDelay())
		s := stopwatch.New()
		s.Start()
		resp, err := r.client.Do(req)
		s.Stop()
		r.progressBar.Increment()
		if err != nil {
			responses <- err
			continue
		}
		if err := r.validate(resp); err != nil {
			responses <- err
			continue
		}
		responses <- s.Duration()
	}

}

func (r *Request) validate(resp *http.Response) error {

	if resp == nil {
		panic("this resp is nil")
	}
	for _, fn := range r.expectations {
		if err := fn(resp); err != nil {
			return err
		}
	}
	return nil
}
