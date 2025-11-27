package hypertext

func (data *TemplateData[R, T]) HXLocation(link string) *TemplateData[R, T] {
	return data.Header("HX-Location", link)
}

func (data *TemplateData[R, T]) HXPushURL(link string) *TemplateData[R, T] {
	return data.Header("HX-Push-Url", link)
}

func (data *TemplateData[R, T]) HXRedirect(link string) *TemplateData[R, T] {
	return data.Header("HX-Redirect", link)
}

func (data *TemplateData[R, T]) HXRefresh() *TemplateData[R, T] {
	return data.Header("HX-Refresh", "true")
}

func (data *TemplateData[R, T]) HXReplaceURL(link string) *TemplateData[R, T] {
	return data.Header("HX-Replace-Url", link)
}

func (data *TemplateData[R, T]) HXReswap(swap string) *TemplateData[R, T] {
	return data.Header("HX-Reswap", swap)
}

func (data *TemplateData[R, T]) HXRetarget(target string) *TemplateData[R, T] {
	return data.Header("HX-Retarget", target)
}

func (data *TemplateData[R, T]) HXReselect(selector string) *TemplateData[R, T] {
	return data.Header("HX-Reselect", selector)
}

func (data *TemplateData[R, T]) HXTrigger(eventName string) *TemplateData[R, T] {
	return data.Header("HX-Trigger", eventName)
}

func (data *TemplateData[R, T]) HXTriggerAfterSettle(eventName string) *TemplateData[R, T] {
	return data.Header("HX-Trigger-After-Settle", eventName)
}

func (data *TemplateData[R, T]) HXTriggerAfterSwap(eventName string) *TemplateData[R, T] {
	return data.Header("HX-Trigger-After-Swap", eventName)
}

func (data *TemplateData[R, T]) HXBoosted() bool {
	return data.Request().Header.Get("HX-Boosted") != ""
}

func (data *TemplateData[R, T]) HXCurrentURL() string {
	return data.Request().Header.Get("HX-Current-Url")
}

func (data *TemplateData[R, T]) HXHistoryRestoreRequest() bool {
	return data.Request().Header.Get("HX-History-Restore-Request") == "true"
}

func (data *TemplateData[R, T]) HXPrompt() string {
	return data.Request().Header.Get("HX-Prompt")
}

func (data *TemplateData[R, T]) HXRequest() bool {
	return data.Request().Header.Get("HX-Request") == "true"
}

func (data *TemplateData[R, T]) HXTargetElementID() string {
	return data.Request().Header.Get("HX-Target")
}

func (data *TemplateData[R, T]) HXTriggerName() string {
	return data.Request().Header.Get("HX-Trigger-Name")
}

func (data *TemplateData[R, T]) HXTriggerElementID() string {
	return data.Request().Header.Get("HX-Trigger")
}
