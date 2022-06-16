package utils

import "fmt"

type Worker struct {
	events     chan struct{}
	workerFunc func() error
}

func NewWorker() *Worker {
	return &Worker{
		events: make(chan struct{}),
	}
}

func (w *Worker) Do() {
	select {
	case w.events <- struct{}{}:
	default:
	}
}

func (w *Worker) Listen() error {
	if w.workerFunc != nil {
		return w.workerFunc()
	}
	return fmt.Errorf("worker function is not registered")
}

func (w *Worker) RegisterFunc(executor func() error) {
	w.workerFunc = func() error {
		for range w.events {
			if err := executor(); err != nil {
				return err
			}
		}
		return nil
	}
}

func (w *Worker) Stop() {
	close(w.events)
}
