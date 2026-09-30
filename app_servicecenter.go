package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"opscopilot/pkg/servicecenter"
)

func (a *App) initServiceCenter() {
	a.serviceCenter = servicecenter.NewClient(filepath.Join(a.configMgr.Directory(), "service-center.json"), Version)
	go a.serviceCenter.Run(a.ctx)
}
func (a *App) GetServiceCenterSettings() servicecenter.Settings {
	if a.serviceCenter == nil {
		return servicecenter.Settings{NeedsConsent: true, Policy: servicecenter.Policy{Version: servicecenter.PolicyVersion, Notice: servicecenter.Notice}}
	}
	return a.serviceCenter.Settings()
}
func (a *App) TestServiceCenterConnection(base string) error {
	if a.serviceCenter == nil {
		return fmt.Errorf("服务尚未初始化")
	}
	return a.serviceCenter.TestConnection(a.ctx, base)
}
func (a *App) ConfigureServiceCenter(base string) (servicecenter.Settings, error) {
	if a.serviceCenter == nil {
		return servicecenter.Settings{}, fmt.Errorf("服务尚未初始化")
	}
	s, e := a.serviceCenter.Configure(base)
	if e == nil {
		go a.serviceCenter.Refresh(a.ctx)
	}
	return s, e
}
func (a *App) SetReportingChoice(choice string) (servicecenter.Settings, error) {
	if a.serviceCenter == nil {
		return servicecenter.Settings{}, fmt.Errorf("服务尚未初始化")
	}
	s, e := a.serviceCenter.Choose(choice)
	if e == nil && (choice == "minimal" || choice == "standard") {
		go a.serviceCenter.Refresh(a.ctx)
	}
	return s, e
}
func (a *App) CountServiceUsage(kind string) {
	if a.serviceCenter != nil && (kind == "ctrl_k" || kind == "quick_command" || (servicecenter.ValidUsage(kind) && len(kind) > 4 && kind[:4] == "gui_")) {
		a.serviceCenter.Count(kind)
	}
}
func (a *App) GetServiceAnnouncements() []servicecenter.Announcement {
	if a.serviceCenter == nil {
		return []servicecenter.Announcement{}
	}
	return a.serviceCenter.Announcements(context.Background())
}
func (a *App) OpenServicePage(page string) error {
	base := a.GetServiceCenterSettings().BaseURL
	if base == "" {
		return fmt.Errorf("请先在设置中配置内网服务地址")
	}
	switch page {
	case "", "help", "feedback":
	default:
		return fmt.Errorf("无效页面")
	}
	runtime.BrowserOpenURL(a.ctx, base+"/"+page)
	return nil
}
func (a *App) OpenServiceAnnouncement(link string) error {
	base := a.GetServiceCenterSettings().BaseURL
	if !servicecenter.InternalURL(base, link) {
		return fmt.Errorf("公告链接必须指向内网服务")
	}
	runtime.BrowserOpenURL(a.ctx, link)
	return nil
}
func (a *App) beginUsage(prefix string) func(string) {
	if a.serviceCenter == nil {
		return func(string) {}
	}
	return a.serviceCenter.BeginUsage(prefix)
}
