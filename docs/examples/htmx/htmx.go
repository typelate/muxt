package hypertext

func (data *TemplateData[T]) HXLocation(link string) *TemplateData[T] {
	data.response.Header().Set("HX-Location", link)
	return data
}

func (data *TemplateData[T]) HXPushURL(link string) *TemplateData[T] {
	data.response.Header().Set("HX-Push-Url", link)
	return data
}

func (data *TemplateData[T]) HXRedirect(link string) *TemplateData[T] {
	data.response.Header().Set("HX-Redirect", link)
	return data
}

func (data *TemplateData[T]) HXRefresh() *TemplateData[T] {
	data.response.Header().Set("HX-Refresh", "true")
	return data
}

func (data *TemplateData[T]) HXReplaceURL(link string) *TemplateData[T] {
	data.response.Header().Set("HX-Replace-Url", link)
	return data
}

func (data *TemplateData[T]) HXReswap(swap string) *TemplateData[T] {
	data.response.Header().Set("HX-Reswap", swap)
	return data
}

func (data *TemplateData[T]) HXRetarget(target string) *TemplateData[T] {
	data.response.Header().Set("HX-Retarget", target)
	return data
}

func (data *TemplateData[T]) HXReselect(selector string) *TemplateData[T] {
	data.response.Header().Set("HX-Reselect", selector)
	return data
}

func (data *TemplateData[T]) HXTrigger(eventName string) *TemplateData[T] {
	data.response.Header().Set("HX-Trigger", eventName)
	return data
}

func (data *TemplateData[T]) HXTriggerAfterSettle(eventName string) *TemplateData[T] {
	data.response.Header().Set("HX-Trigger-After-Settle", eventName)
	return data
}

func (data *TemplateData[T]) HXTriggerAfterSwap(eventName string) *TemplateData[T] {
	data.response.Header().Set("HX-Trigger-After-Swap", eventName)
	return data
}

func (data *TemplateData[T]) HXBoosted() bool {
	return data.request.Header.Get("HX-Boosted") != ""
}

func (data *TemplateData[T]) HXCurrentURL() string {
	return data.request.Header.Get("HX-Current-Url")
}

func (data *TemplateData[T]) HXHistoryRestoreRequest() bool {
	return data.request.Header.Get("HX-History-Restore-Request") == "true"
}

func (data *TemplateData[T]) HXPrompt() string {
	return data.request.Header.Get("HX-Prompt")
}

func (data *TemplateData[T]) HXRequest() bool {
	return data.request.Header.Get("HX-Request") == "true"
}

func (data *TemplateData[T]) HXTargetElementID() string {
	return data.request.Header.Get("HX-Target")
}

func (data *TemplateData[T]) HXTriggerName() string {
	return data.request.Header.Get("HX-Trigger-Name")
}

func (data *TemplateData[T]) HXTriggerElementID() string {
	return data.request.Header.Get("HX-Trigger")
}
