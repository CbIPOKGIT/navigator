package navigator

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-rod/rod"
)

// stepAuthorize виконує авторизацію у ChromeNavigator, якщо вона налаштована в моделі і ще не виконувалась для поточного клієнта.
func (n *ChromeNavigator) stepAuthorize(ctx context.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("authorization panic")
		}
	}()

	if !n.needAuthorization() {
		return nil
	}

	page := n.getPage()
	if page == nil {
		return errors.New("page is closed")
	}

	auth := n.Model.Authorization
	if auth == nil || !auth.Use {
		return nil
	}

	var reloadCh chan error
	var cancelReload context.CancelFunc
	if auth.WaitReload {
		reloadCh = make(chan error, 1)
		var reloadCtx context.Context
		reloadCtx, cancelReload = context.WithCancel(ctx)
		defer cancelReload()

		go func() {
			reloadCh <- n.WaitResponseAndLoad(reloadCtx)
		}()
	}

	if auth.UseScript {
		err = n.authorizeChromeWithScript(ctx, page, auth)
	} else {
		err = n.authorizeChromeWithForm(ctx, page, auth)
	}

	if err != nil {
		if cancelReload != nil {
			cancelReload()
		}
		return err
	}

	n.isAuthorized.Store(true)

	if auth.WaitReload {
		select {
		case <-ctx.Done():
			return errors.New("navigation cancelled")
		case reloadErr := <-reloadCh:
			if reloadErr != nil {
				return reloadErr
			}
		}

		// Звіряємо, чи співпадає поточне посилання з цільовим
		actualUrl := n.GetActualUrl()
		targetUrl := n.GetUrl()
		if actualUrl != "" && actualUrl != targetUrl {
			log.Printf("URL after authorization (%s) differs from target (%s), navigating back", actualUrl, targetUrl)
			n.setExecutorStep(EXECUTOR_GOTO_URL)
		}
	}

	return nil
}

func (n *ChromeNavigator) authorizeChromeWithScript(ctx context.Context, page *rod.Page, auth *Authorization) error {
	if auth.AuthorizationScript == nil || strings.TrimSpace(*auth.AuthorizationScript) == "" {
		return errors.New("authorization script is empty")
	}

	script := fmt.Sprintf(`async () => {
		%s
	}`, *auth.AuthorizationScript)

	_, err := page.Context(ctx).Eval(script)
	return err
}

func (n *ChromeNavigator) authorizeChromeWithForm(ctx context.Context, page *rod.Page, auth *Authorization) error {
	formSelector := auth.FormSelector
	if formSelector == "" {
		formSelector = "form"
	}

	formEl, err := page.Context(ctx).Timeout(time.Second * 10).Element(formSelector)
	if err != nil {
		return fmt.Errorf("authorization form not found (%s): %w", formSelector, err)
	}

	for _, field := range auth.Fields {
		if field == nil || field.Name == "" {
			continue
		}

		inputSelector := fmt.Sprintf("input[name=%q]", field.Name)
		inputEl, err := formEl.Element(inputSelector)
		if err != nil {
			// Fallback: шукаємо поле на всій сторінці, якщо воно поза тегом form
			inputEl, err = page.Context(ctx).Timeout(time.Second * 5).Element(inputSelector)
			if err != nil {
				return fmt.Errorf("authorization input %s not found: %w", inputSelector, err)
			}
		}

		_, err = inputEl.Eval(`(val) => {
			this.value = val;
			this.dispatchEvent(new Event('input', { bubbles: true }));
			this.dispatchEvent(new Event('change', { bubbles: true }));
		}`, field.Value)
		if err != nil {
			return fmt.Errorf("failed to set value for input %s: %w", field.Name, err)
		}
	}

	if auth.SubmitAddr != "" {
		submitUrl := n.FormatUrl(auth.SubmitAddr)
		if _, err := formEl.Eval(`(action) => { this.action = action; }`, submitUrl); err != nil {
			return fmt.Errorf("failed to set form action: %w", err)
		}
	}

	_, err = formEl.Eval(`() => {
		if (typeof this.requestSubmit === 'function') {
			this.requestSubmit();
		} else {
			this.submit();
		}
	}`)
	if err != nil {
		return fmt.Errorf("failed to submit authorization form: %w", err)
	}

	return nil
}

// authorizeResty виконує авторизацію через відправку форми в GentelmanNavigator (Resty).
func (navigator *GentelmanNavigator) authorizeResty() error {
	if !navigator.needAuthorization() {
		return nil
	}

	auth := navigator.Model.Authorization
	if auth == nil || !auth.Use {
		return nil
	}

	if auth.UseScript {
		return errors.New("script authorization is not supported in resty navigator")
	}

	formData := make(map[string]string)
	submitAddr := auth.SubmitAddr

	if auth.FormSelector != "" && navigator.Crawler != nil {
		form := navigator.Crawler.Find(auth.FormSelector).First()
		if form.Length() > 0 {
			form.Find("input").Each(func(_ int, s *goquery.Selection) {
				if name, ok := s.Attr("name"); ok && name != "" {
					val, _ := s.Attr("value")
					formData[name] = val
				}
			})

			if submitAddr == "" {
				if action, ok := form.Attr("action"); ok && action != "" {
					submitAddr = action
				}
			}
		}
	}

	for _, field := range auth.Fields {
		if field != nil && field.Name != "" {
			formData[field.Name] = field.Value
		}
	}

	var submitUrl string
	if submitAddr != "" {
		submitUrl = navigator.FormatUrl(submitAddr)
	} else {
		submitUrl = navigator.GetUrl()
	}

	postResp, err := navigator.Client.R().
		SetFormData(formData).
		Post(submitUrl)
	if err != nil {
		log.Println("Error submitting resty authorization form:", err)
		navigator.logRequest(0, err)
		return errors.New("error navigate")
	}

	statusCode := int32(postResp.StatusCode())
	navigator.NavigateStatus.Store(statusCode)
	navigator.logRequest(statusCode, nil)

	navigator.isAuthorized.Store(true)

	actualUrl := submitUrl
	if postResp.RawResponse != nil && postResp.RawResponse.Request != nil && postResp.RawResponse.Request.URL != nil {
		actualUrl = postResp.RawResponse.Request.URL.String()
	}

	if auth.WaitReload {
		if actualUrl == navigator.GetUrl() {
			if err := navigator.СreateCrawlerFromHTML(postResp.String()); err != nil {
				return fmt.Errorf("error create crawler from HTML: %s", err.Error())
			}
		} else {
			getResp, err := navigator.Client.R().Get(navigator.Uri.String())
			if err != nil {
				log.Println(err)
				navigator.logRequest(0, err)
				return errors.New("error navigate")
			}

			getStatus := int32(getResp.StatusCode())
			navigator.NavigateStatus.Store(getStatus)
			navigator.logRequest(getStatus, nil)

			if err := navigator.СreateCrawlerFromHTML(getResp.String()); err != nil {
				return fmt.Errorf("error create crawler from HTML: %s", err.Error())
			}
		}
	}

	return nil
}
