package navigator

import (
	"fmt"
	"log"
)

func handleErrorWithErrorChan(errChan chan error) {
	if err := recover(); err != nil {
		log.Printf("Panic: %v", err)

		if errChan != nil {
			if errData, is := err.(error); is {
				errChan <- errData
			} else {
				errChan <- fmt.Errorf("panic: %v", err)
			}
		}

	}
}
