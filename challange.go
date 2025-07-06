package navigator

import (
	"errors"
	"log"
	"time"

	"github.com/go-rod/rod"
	"golang.org/x/net/context"
)

const (
	CHALLENGE_SOLVE_DURATION = time.Minute * 3
)

// CloudflareSolver - interface for Cloudflare protection solver
type CloudflareSolver interface {
	Is(*rod.Page) bool // check if page has Cloudflare protection

	Solve(*rod.Page, ...context.Context) error // solve Cloudflare protection

	SetSitekeyScript(string) // set script to get sitekey

	SetSolveScript(string) // set script to solve challenge

	SetReloadFunction(func() error) // set function to reload page
}

// beatChallange - beat the challange. Its something like Cloudflare protection.
//
// Max reloads count - 5
func (n *ChromeNavigator) beatChallange(response ...StateChannel) error {
	state := &ChromeStateStatus{
		Step: NAVIGATION_STATE_CHALLANGE_SOLVED,
	}

	defer writeToChromeStatusChannel(state, response...)

	if n.ClfSolver != nil && n.ClfSolver.Is(n.Page) {
		state.Error = n.solveWithClfSolver()
	} else {
		state.Error = n.solveSimpleChallange()
	}
	return state.Error
}

func (n *ChromeNavigator) solveWithClfSolver() error {
	errChannel := make(chan error, 1)

	go func() {
		errChannel <- n.ClfSolver.Solve(n.Page, n.context)
	}()

	select {
	case err := <-errChannel:
		if err == nil {
			log.Println("Challenge solved with Cloudflare solver")
		} else {
			log.Println("Error solving challenge with Cloudflare solver:", err)
		}
		return err
	case <-n.context.Done():
		return errors.New("context canceled")

	}
}

func (n *ChromeNavigator) solveSimpleChallange() error {
	if !n.hasChallenge() {
		log.Println("No challange")
		return nil
	}

	log.Println("Challenge detected, starting to solve...")
	maxStepCount := 5
	reloaded := make(chan error)
	contextChallenge, _ := context.WithTimeout(n.context, CHALLENGE_SOLVE_DURATION)

	reloadFunc := func() { reloaded <- n.WaitResponseAndLoad(contextChallenge) }
	go reloadFunc()

	for {
		select {
		case <-contextChallenge.Done():
			return errors.New("timeout challenge solve")

		case err := <-reloaded:
			if err != nil {
				log.Println("Error reloading page:", err)
				return err
			}

			if !n.hasChallenge() {
				log.Println("Challenge solved")
				return nil
			}

			maxStepCount--
			if maxStepCount <= 0 {
				log.Println("Max step count reached. Challenge not solved.")
				return errors.New("max step count reached")
			} else {
				log.Printf("Challenge not solved. Step %d/%d", 5-maxStepCount, 5)
				go reloadFunc()
			}
		}
	}
}

func (n *ChromeNavigator) hasChallenge() bool {
	return n.Model.ChallangeSelector != "" && n.hasChallengeElement(n.Model.ChallangeSelector)
}

func (n *ChromeNavigator) hasChallengeElement(selector string) bool {
	elements, err := n.Page.Elements(selector)
	return err == nil && len(elements) > 0
}
