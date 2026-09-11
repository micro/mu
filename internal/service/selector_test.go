package service

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"go-micro.dev/v6/registry"
	"go-micro.dev/v6/selector"
)

func TestConcurrentFirstMemoryLookupsFindRegisteredServices(t *testing.T) {
	r := registry.NewMemoryRegistry()
	s := serviceSelector(r)
	defer s.Close()
	for i := 0; i < 40; i++ {
		name := fmt.Sprintf("cold-%d", i)
		service := &registry.Service{Name: name, Version: "1", Nodes: []*registry.Node{{Id: name, Address: "127.0.0.1:1234"}}}
		if err := r.Register(service); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		start := make(chan struct{})
		for j := 0; j < 16; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				next, err := s.Select(name)
				if err != nil {
					t.Error(err)
					return
				}
				node, err := next()
				if err != nil || node == nil || node.Id != name {
					t.Errorf("missing registered node: %v, %v", node, err)
				}
			}()
		}
		close(start)
		wg.Wait()
		if _, err := s.Select(name, selector.WithFilter(func([]*registry.Service) []*registry.Service { return nil })); !errors.Is(err, selector.ErrNoneAvailable) {
			t.Fatalf("filter ignored: %v", err)
		}
		if err := r.Deregister(service); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Select(name); !errors.Is(err, selector.ErrNotFound) {
			t.Fatalf("removed service remained available: %v", err)
		}
	}
}
