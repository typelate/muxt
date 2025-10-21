package hypertext

func (data *TemplateData[R, T]) HXLocation(link string) *TemplateData[R, T] {
	data.response.Header().Set("HX-Location", link)
	return data
}

func (data *TemplateData[R, T]) HXPushURL(link string) *TemplateData[R, T] {
	data.response.Header().Set("HX-Push-Url", link)
	return data
}

func (data *TemplateData[R, T]) HXRedirect(link string) *TemplateData[R, T] {
	data.response.Header().Set("HX-Redirect", link)
	return data
}

func (data *TemplateData[R, T]) HXRefresh() *TemplateData[R, T] {
	data.response.Header().Set("HX-Refresh", "true")
	return data
}

func (data *TemplateData[R, T]) HXReplaceURL(link string) *TemplateData[R, T] {
	data.response.Header().Set("HX-Replace-Url", link)
	return data
}

func (data *TemplateData[R, T]) HXReswap(swap string) *TemplateData[R, T] {
	data.response.Header().Set("HX-Reswap", swap)
	return data
}

func (data *TemplateData[R, T]) HXRetarget(target string) *TemplateData[R, T] {
	data.response.Header().Set("HX-Retarget", target)
	return data
}

func (data *TemplateData[R, T]) HXReselect(selector string) *TemplateData[R, T] {
	data.response.Header().Set("HX-Reselect", selector)
	return data
}

func (data *TemplateData[R, T]) HXTrigger(eventName string) *TemplateData[R, T] {
	data.response.Header().Set("HX-Trigger", eventName)
	return data
}

func (data *TemplateData[R, T]) HXTriggerAfterSettle(eventName string) *TemplateData[R, T] {
	data.response.Header().Set("HX-Trigger-After-Settle", eventName)
	return data
}

func (data *TemplateData[R, T]) HXTriggerAfterSwap(eventName string) *TemplateData[R, T] {
	data.response.Header().Set("HX-Trigger-After-Swap", eventName)
	return data
}

func (data *TemplateData[R, T]) HXBoosted() bool {
	return data.request.Header.Get("HX-Boosted") != ""
}

func (data *TemplateData[R, T]) HXCurrentURL() string {
	return data.request.Header.Get("HX-Current-Url")
}

func (data *TemplateData[R, T]) HXHistoryRestoreRequest() bool {
	return data.request.Header.Get("HX-History-Restore-Request") == "true"
}

func (data *TemplateData[R, T]) HXPrompt() string {
	return data.request.Header.Get("HX-Prompt")
}

func (data *TemplateData[R, T]) HXRequest() bool {
	return data.request.Header.Get("HX-Request") == "true"
}

func (data *TemplateData[R, T]) HXTargetElementID() string {
	return data.request.Header.Get("HX-Target")
}

func (data *TemplateData[R, T]) HXTriggerName() string {
	return data.request.Header.Get("HX-Trigger-Name")
}

func (data *TemplateData[R, T]) HXTriggerElementID() string {
	return data.request.Header.Get("HX-Trigger")
}
