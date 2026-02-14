// Permission to use, copy, modify, and/or distribute this software for
// any purpose with or without fee is hereby granted.
//
// THE SOFTWARE IS PROVIDED “AS IS” AND THE AUTHOR DISCLAIMS ALL
// WARRANTIES WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES
// OF MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE
// FOR ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY
// DAMAGES WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN
// AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT
// OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.

package assets

import (
	"embed"
)

//go:embed html/home.html
var HtmlHome string

//go:embed html/login.html
var HtmlLogin string

//go:embed html/ctl-user.html
var HtmlCtlUser string

//go:embed html/object-repo.html
var HtmlObjectRepo string

//go:embed html/object-ticket-tracker.html
var HtmlObjectTicketTracker string

//go:embed html/object-ticket.html
var HtmlObjectTicket string

//go:embed html/object-ticket-comment.html
var HtmlObjectTicketComment string

//go:embed html/data-list.html
var HtmlDataList string

//go:embed html/data-project.html
var HtmlDataProject string

//go:embed html/ticket-tracker-list.html
var HtmlTicketTrackerList string

//go:embed html/ticket-tracker-detail.html
var HtmlTicketTrackerDetail string

//go:embed html/ticket-list.html
var HtmlTicketList string

//go:embed html/notification-list.html
var HtmlNotificationList string

//go:embed html/oauth-consent.html
var HtmlOAuthConsent string

//go:embed html/oauth-grant-list.html
var HtmlOAuthGrantList string

//go:embed sql/create.sql
var SqlControlCreate string

//go:embed js
var JsFiles embed.FS
