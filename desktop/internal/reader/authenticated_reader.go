package reader

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"

	"taskpicker/internal/model"
	"taskpicker/internal/storage"
)

// Login uses an application-owned Chrome profile. Cookies remain on this Mac;
// the user's normal browser profile is never inspected or copied.
func ReadAuthenticatedTasks(ctx context.Context, link, owner string, notify ProgressFunc, includeCompleted bool, options ...ReadOptions) (model.Document, error) {
	if err := ctx.Err(); err != nil {
		return model.Document{}, err
	}
	profile := filepath.Join(filepath.Dir(storage.LibraryRoot()), "browser")
	if err := os.MkdirAll(profile, 0700); err != nil {
		return model.Document{}, err
	}
	browserOptions := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	browserOptions = append(browserOptions, chromedp.Flag("headless", false), chromedp.UserDataDir(profile), chromedp.Flag("no-first-run", true))
	allocator, stopAllocator := chromedp.NewExecAllocator(ctx, browserOptions...)
	defer stopAllocator()
	browser, stopBrowser := chromedp.NewContext(allocator)
	defer stopBrowser()
	notify("文档需要登录，正在打开独立的 Chrome 登录窗口…")
	if err := chromedp.Do(browser); err != nil {
		return model.Document{}, fmt.Errorf("登录窗口无法打开。请安装 Google Chrome，并关闭本工具先前打开的登录窗口后重试：%w", err)
	}
	navigation, cancelNavigation := context.WithTimeout(browser, 60*time.Second)
	err := chromedp.Do(navigation, chromedp.Navigate(link))
	cancelNavigation()
	if err != nil {
		if ctx.Err() != nil {
			return model.Document{}, ctx.Err()
		}
		return model.Document{}, fmt.Errorf("登录窗口无法打开。请安装 Google Chrome，并关闭本工具先前打开的登录窗口后重试：%w", err)
	}
	notify("请在新窗口登录有查看权限的账号，登录后会自动读取。也可取消后导入 Excel。")
	target, _ := url.Parse("https://docs.qq.com")
	client := NewHTTPClient()
	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()
	deadline := time.NewTimer(20 * time.Minute)
	defer deadline.Stop()
	for {
		select {
		case <-ctx.Done():
			return model.Document{}, ctx.Err()
		case <-browser.Done():
			return model.Document{}, errors.New("登录窗口已关闭，已取消读取。")
		case <-deadline.C:
			return model.Document{}, errors.New("等待登录超时，请重新同步。")
		case <-ticker.C:
			probe, stop := context.WithTimeout(browser, 25*time.Second)
			response, err := chromedp.Call(probe, network.GetCookies, network.GetCookiesParams{URLs: []string{link}})
			stop()
			if err != nil {
				if ctx.Err() != nil {
					return model.Document{}, ctx.Err()
				}
				return model.Document{}, errors.New("登录窗口已关闭或失去连接，请重新同步。")
			}
			var converted []*http.Cookie
			for _, cookie := range response.Cookies {
				converted = append(converted, &http.Cookie{Name: cookie.Name, Value: cookie.Value, Domain: cookie.Domain, Path: cookie.Path, Secure: cookie.Secure, HttpOnly: cookie.HTTPOnly})
			}
			client.Jar.SetCookies(target, converted)
			doc, err := ReadPublicTasks(ctx, link, owner, func(string) {}, client, includeCompleted, options...)
			if err == nil {
				notify("已登录，任务和图片已读取。正在保存…")
				return doc, nil
			}
			if !errors.Is(err, ErrNeedsLogin) {
				return model.Document{}, err
			}
		}
	}
}
