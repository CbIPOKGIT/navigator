package navigator

import (
	"encoding/json"
	"errors"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/go-rod/rod/lib/proto"
)

const (
	CLOUDFLARE_CHALLENGE_SELECTOR = "#cf-wrapper, #turnstile-wrapper"
	CLOUDFLARE_ENV_VARIABLE       = "CLOUDFLARE_CHALLENGE_SELECTOR"
	CHALLENGE_SOLVE_DURATION      = time.Minute * 3
)

// beatChallange - beat the challange. Its something like Cloudflare protection.
//
// Max reloads count - 5
func (navigator *ChromeNavigator) beatChallange() error {
	time.Sleep(time.Millisecond * 100)

	if !navigator.hasChallenge() {
		return nil
	}

	isCloudflare := navigator.hasCloudflareChallenge()

	reloaded := make(chan any, 1)
	var reloaderCounter int

	reloader := func() {
		navigator.waitResponseAndLoad()
		log.Println("Reloaded")
		reloaded <- nil
	}
	go reloader()

	solveTimeout := time.NewTimer(CHALLENGE_SOLVE_DURATION)

	stopSolvingCloudflare := make(chan any, 1)

	if isCloudflare {
		go navigator.solveCloudflareChallenge(stopSolvingCloudflare)
	}

	for {
		select {
		case <-reloaded:

			if isCloudflare {
				stopSolvingCloudflare <- nil
			}

			if !navigator.hasChallenge() {
				return nil
			}

			if isCloudflare {
				go navigator.solveCloudflareChallenge(stopSolvingCloudflare)
			}

			reloaderCounter++

			if reloaderCounter < 4 {
				go reloader()
				continue
			} else {
				return errors.New("unable pass challange form")
			}

		case <-solveTimeout.C:
			return errors.New("timeout challange")

		}
	}

}

func (navigator *ChromeNavigator) solveCloudflareChallenge(stopSolving chan any) {
	resetTicker := time.NewTicker(time.Second * 15)

	clickTimer := time.NewTimer(time.Second * 5)
	clickTimer.Stop()

	for {
		select {
		case <-stopSolving:
			return

		case <-resetTicker.C:

			navigator.Page.Activate()
			navigator.Page.MustEval("() => window.turnstile.reset()")

			clickTimer.Reset(time.Second * 5)
			log.Println("Reset")

		case <-clickTimer.C:

			navigator.Page.Activate()
			resp, err := navigator.Page.Eval("() => JSON.stringify(document.querySelector('iframe').getBoundingClientRect())")
			if err != nil {
				continue
			}

			coords := make(map[string]float64, 4)

			if err := json.Unmarshal([]byte(resp.Value.Str()), &coords); err != nil {
				continue
			}

			navigator.Page.Mouse.MoveLinear(
				proto.Point{
					X: coords["x"] + 20 + float64(rand.Intn(40)),
					Y: coords["y"] + 20 + float64(rand.Intn(40)),
				},
				10+rand.Intn(30),
			)
			navigator.Page.Mouse.MoveLinear(proto.Point{X: coords["x"] + 20, Y: coords["y"] + 20}, 10+rand.Intn(10))
			navigator.Page.Mouse.MustClick(proto.InputMouseButtonLeft)
			log.Println("Click")
		}
	}
}

// hasChallenge - check if page has challenge.
//
// Checking for selector from model and Cloudflare by default
func (navigator *ChromeNavigator) hasChallenge() bool {
	return navigator.hasModelChallenge() || navigator.hasCloudflareChallenge()
}

// hasModelChallange - check if page has challenge by model selector
func (navigator *ChromeNavigator) hasModelChallenge() bool {
	if navigator.Model.ChallangeSelector == "" {
		return false
	}
	return navigator.hasChallengeBySelector(navigator.Model.ChallangeSelector)
}

func (navigator *ChromeNavigator) hasCloudflareChallenge() bool {
	time.Sleep(time.Millisecond * 200)
	if value := os.Getenv(CLOUDFLARE_ENV_VARIABLE); value != "" {
		if navigator.hasChallengeBySelector(value) {
			return true
		}
	}
	return navigator.hasChallengeBySelector(CLOUDFLARE_CHALLENGE_SELECTOR)
}

func (navigator *ChromeNavigator) hasChallengeBySelector(selector string) bool {
	elements, err := navigator.Page.Elements(selector)
	return err == nil && len(elements) > 0
}
