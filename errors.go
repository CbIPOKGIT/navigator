package navigator

import "fmt"

func handleErrorWithErrorChan(errChan chan error) {
	if err := recover(); err != nil && errChan != nil {
		if errData, is := err.(error); is {
			errChan <- errData
		} else {
			errChan <- fmt.Errorf("Panic: %v", err)
		}
	}
}

func handleErrorWithAnyChan(errChan chan any) {
	if err := recover(); err != nil && errChan != nil {
		errChan <- err
	}
}
