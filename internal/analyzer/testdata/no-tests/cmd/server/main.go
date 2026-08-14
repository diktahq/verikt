package main

// server has a Run method. A bare ".Run(" call is not a subtest — this fixture
// has no test files at all, so no testing pattern should be inferred from it.
type server struct{}

func (s *server) Run() {}

func main() {
	s := &server{}
	s.Run()
}
