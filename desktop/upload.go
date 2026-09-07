package main

// The upload direction of the same problem reports.go solves for downloads.
//
// Everything the window fetches is intercepted by WebView2 and handed to Wails,
// which rebuilds it into an http.Request from an opaque IStream before the
// proxy forwards it to the server (assetserver_webview.go). A GET survives that
// intact. A multipart POST carrying half a megabyte of DOCX does not: the
// server saw the file part arrive with no content in it, so the extractor was
// handed an empty document and reported that a valid report was not a valid
// report. Seven attempts, seven rejections, and never a hint that the file had
// been lost rather than misread.
//
// Requests to an absolute loopback URL are not intercepted at all - only the
// app's own wails:// origin is - so an upload sent straight at the server
// bypasses the whole mechanism. ServerURL hands the page that address; api.js
// uses it for multipart uploads and leaves everything else on the proxied path
// that already works.

// ServerURL is the address of the supervised server, e.g. http://127.0.0.1:52413.
//
// The port is chosen per run by freePort(), so the page cannot know it without
// asking. Returning it lets the web app post a file directly at the server
// instead of through the window's asset handler.
func (a *App) ServerURL() string {
	return a.baseURL
}
