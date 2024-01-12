package navigator

import (
	"fmt"
	"log"
)

func handleErrorWithErrorChan(errChan chan error, description string) {
	if err := recover(); err != nil {
		log.Printf("Panic %s: %v", description, err)

		if errChan != nil {
			if errData, is := err.(error); is {
				errChan <- errData
			} else {
				errChan <- fmt.Errorf("Panic: %v", err)
			}
		}

	}
}

func handleErrorWithAnyChan(errChan chan any, description string) {
	if err := recover(); err != nil {

		log.Printf("Panic %s: %v", description, err)

		if errChan != nil {
			errChan <- err
		}
	}
}
