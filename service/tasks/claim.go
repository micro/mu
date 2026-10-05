package tasks

import "fmt"

// Claims are process-local execution ownership. Persisted attempts and event
// receipts retain crash history; a restart never automatically retries effects.
var claims = map[string]chan struct{}{}

// Claim reserves the current attempt for one worker. Stop closes the returned
// channel. The worker must release only after its execution has actually ended.
func Claim(owner, id, attempt string) (<-chan struct{}, func(), error) {
	runMu.Lock()
	defer runMu.Unlock()
	t, err := Get(owner, id)
	if err != nil {
		return nil, nil, err
	}
	key := owner + ":" + id
	if t.Status != StatusDoing || len(t.Attempts) == 0 || t.Attempts[len(t.Attempts)-1].ID != attempt || claims[key] != nil {
		return nil, nil, fmt.Errorf("work attempt is no longer available")
	}
	stopped := make(chan struct{})
	claims[key] = stopped
	return stopped, func() {
		runMu.Lock()
		defer runMu.Unlock()
		if claims[key] == stopped {
			delete(claims, key)
		}
	}, nil
}

func stopClaim(owner, id string) {
	if ch := claims[owner+":"+id]; ch != nil {
		select {
		case <-ch:
		default:
			close(ch)
		}
	}
}

// Preparation activity distinguishes a live occurrence from an interrupted one.
var preparations = map[string]int{}

func BeginPreparation(owner, key string) func() {
	runMu.Lock()
	preparations[owner+":"+key]++
	runMu.Unlock()
	return func() {
		runMu.Lock()
		defer runMu.Unlock()
		k := owner + ":" + key
		preparations[k]--
		if preparations[k] <= 0 {
			delete(preparations, k)
		}
	}
}

func Preparing(owner, key string) bool {
	runMu.Lock()
	defer runMu.Unlock()
	return preparations[owner+":"+key] > 0
}
