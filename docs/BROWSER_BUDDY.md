# Browswer Buddy -- A Chrome Extension for Virtual Agents

## Background

Several web sights of interest have rules or systems that prohibit scraping their site from a headless service, notably reddit.com, youtube.com, and news.ycombinator.com (to a lessor degree).
These sites have information of interest that is desired to have local copies of, so we want to be able to find workarounds to acquire this info; for this we will need a Browser Buddy.

Browser Buddy is a Chrome extension (hopefully ported to Firefox in the future -- TBD), that leverages the users authenticated state to make requests on behalf of a backend service that will ultimately consume this info.

## Goals

* Capture cookies and auth state to access protected resources
* Work with a backend system that can access those resources using the shared cookies/auth
* Optionally make the requests within the browser and hand the results of to the backend (in the background, using a service worker)
* Provide a simple UI to expose the state/debugging of the extension for troubleshooting and refinement

IMPORTANT: the browser buddy *MUST* act like a human in its behavior and *NEVER* act in a way that shows it to be anything else but a human using the site;
for this to take place it *MUST* make requests at a speed and cadence that emulates a human (irregular periods between requests, not being too quick as to appear automated, etc)

## Engineering requirements

* All functionality should come with thourough tests that must always be passed for acceptance criteria
* Full debugging capability via logging that can be changed before or during operation (e.g., log levels of INFO, ERROR, DEBUG, etc)
* Use Golang for the backend, following the Go practices defined for you
* Have separation of concerns as much as possible without making it more unwieldy (e.g., fetching, storing, logging, etc)
* In the spirit of searated concerns, try to be as composable as possible, again being mindful of the balance between clean/clear focus and code readability/maintainability

## Caveats

While the end goal is a common platform for multiple sites, it is fine to start off in the browser extension to have unique code paths per site for interacting with the site, but make the backend interaction as unified as possible from the start. Obviously sites will have different data structures but they have a common theme of "pages" or "articles" with details about those articles (e.g., comments, metadata), etc.

Let's make it work first with whatever is the simplest and fastest way across sites and once that is working suitably to try to unifiy it where it makes sense.

## Notes

This project, like every other, is based on assumptions that make sense to me but I am not infallible and if those assumptions or requests are not sensible to you, you *must* make that point and ensure the matter is clarified/resolved before continuing on. Do not be obsequious or overly agreeable -- speak out when you think I'm wrong.
