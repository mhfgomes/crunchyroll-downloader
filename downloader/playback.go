package downloader

import (
	"fmt"
	"sync"
)

// runPlaybackJobs bounds whole playback lifetimes, including opening and
// releasing tokens. Stop scheduling on failure, and join every started job
// before returning so cleanup cannot race the next episode.
func runPlaybackJobs(count, limit int, run func(int) error) error {
	if limit < 1 {
		return fmt.Errorf("-simultaneous must be at least 1")
	}
	var mu sync.Mutex
	next := 0
	var firstErr error
	var wg sync.WaitGroup
	for worker := 0; worker < min(count, limit); worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				mu.Lock()
				if firstErr != nil || next == count {
					mu.Unlock()
					return
				}
				i := next
				next++
				mu.Unlock()
				err := func() (err error) {
					defer func() {
						if r := recover(); r != nil {
							if e, ok := r.(error); ok {
								err = e
							} else {
								err = fmt.Errorf("%v", r)
							}
						}
					}()
					return run(i)
				}()
				mu.Lock()
				if firstErr == nil && err != nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return firstErr
}
