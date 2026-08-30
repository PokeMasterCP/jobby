package main

import (
	"database/sql"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/pokemastercp/jobby/internal/database"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed db/migrations/*.sql web/templates/*.html web/static
var appFS embed.FS

type overviewData struct {
	Applications              []applicationView
	PortalApplications        []portalApplicationView
	ApplicationSummary        string
	PortalHeadingCount        string
	PortalHeadingAction       string
	PortalCheckSummary        string
	PortalDescription         string
	TotalApplications         int
	OrganizationCount         int
	OpenCount                 int
	AppliedCount              int
	InContactCount            int
	AcceptedCount             int
	RejectedAfterContactCount int
	RejectedNoContactCount    int
}

type applicationView struct {
	OrganizationName    string
	OrganizationInitial string
	OrganizationMark    string
	RoleTitle           string
	Salary              string
	Location            string
	LocationIcon        string
	StatusLabel         string
	StatusClass         string
	LastChecked         string
	LastCheckedDetail   string
	LastCheckedIsDue    bool
}

type portalApplicationView struct {
	OrganizationName string
	LastChecked      string
	daysSinceCheck   int
}

func main() {
	db, err := sql.Open("sqlite", "jobby.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		log.Fatal(err)
	}

	goose.SetBaseFS(appFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		log.Fatal(err)
	}
	if err := goose.Up(db, "db/migrations"); err != nil {
		log.Fatal(err)
	}

	queries := database.New(db)

	templates, err := template.ParseFS(appFS, "web/templates/*.html")
	if err != nil {
		log.Fatal(err)
	}

	staticFS, err := fs.Sub(appFS, "web/static")
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		applications, err := queries.ListApplications(r.Context())
		if err != nil {
			log.Printf("list applications: %v", err)
			http.Error(w, "Unable to load applications", http.StatusInternalServerError)
			return
		}

		data := overviewData{
			TotalApplications: len(applications),
		}
		organizations := make(map[int64]struct{})
		now := time.Now()
		for _, application := range applications {
			organizations[application.OrganizationID] = struct{}{}

			switch application.Status {
			case "applied":
				data.AppliedCount++
			case "in_contact":
				data.InContactCount++
			case "accepted":
				data.AcceptedCount++
			case "rejected_after_contact":
				data.RejectedAfterContactCount++
			case "rejected_no_contact":
				data.RejectedNoContactCount++
			}

			if !isOpenStatus(application.Status) {
				continue
			}

			data.OpenCount++
			applicationView := newApplicationView(application, now)
			data.Applications = append(data.Applications, applicationView)
			if applicationView.LastCheckedIsDue {
				data.PortalApplications = append(data.PortalApplications, newPortalApplicationView(application, now))
			}
		}
		data.OrganizationCount = len(organizations)

		data.ApplicationSummary = fmt.Sprintf("Showing all %d open applications", data.OpenCount)
		if data.OpenCount == 1 {
			data.ApplicationSummary = "Showing 1 open application"
		}

		sort.Slice(data.PortalApplications, func(i, j int) bool {
			return data.PortalApplications[i].daysSinceCheck > data.PortalApplications[j].daysSinceCheck
		})
		data.PortalHeadingCount, data.PortalHeadingAction, data.PortalCheckSummary = portalCopy(len(data.PortalApplications))
		data.PortalDescription = "These applications have not been checked in seven or more days."
		if len(data.PortalApplications) == 0 {
			data.PortalDescription = "You're caught up on portal checks for every open application."
		}
		if len(data.PortalApplications) > 3 {
			data.PortalApplications = data.PortalApplications[:3]
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := templates.ExecuteTemplate(w, "index.html", data); err != nil {
			log.Printf("render overview: %v", err)
		}
	})

	addr := ":8080"
	log.Printf("listening on http://localhost%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func isOpenStatus(status string) bool {
	return status == "applied" || status == "in_contact"
}

func newApplicationView(application database.ListApplicationsRow, now time.Time) applicationView {
	lastChecked, lastCheckedDetail, lastCheckedIsDue := formatLastChecked(application.LastCheckedAt, now)
	statusLabel, statusClass := formatStatus(application.Status)

	return applicationView{
		OrganizationName:    application.OrganizationName,
		OrganizationInitial: organizationInitial(application.OrganizationName),
		OrganizationMark:    organizationMark(application.OrganizationID),
		RoleTitle:           application.RoleTitle,
		Salary:              formatSalary(application.SalaryMin, application.SalaryMax),
		Location:            strings.ToUpper(application.WorkLocation[:1]) + application.WorkLocation[1:],
		LocationIcon:        map[string]string{"remote": "⌂", "local": "⌖"}[application.WorkLocation],
		StatusLabel:         statusLabel,
		StatusClass:         statusClass,
		LastChecked:         lastChecked,
		LastCheckedDetail:   lastCheckedDetail,
		LastCheckedIsDue:    lastCheckedIsDue,
	}
}

func newPortalApplicationView(application database.ListApplicationsRow, now time.Time) portalApplicationView {
	if !application.LastCheckedAt.Valid {
		return portalApplicationView{
			OrganizationName: application.OrganizationName,
			LastChecked:      "Never",
			daysSinceCheck:   int(^uint(0) >> 1),
		}
	}

	checkedAt, _ := time.Parse(time.RFC3339Nano, application.LastCheckedAt.String)
	daysSinceCheck := int(now.Sub(checkedAt).Hours() / 24)
	return portalApplicationView{
		OrganizationName: application.OrganizationName,
		LastChecked:      fmt.Sprintf("%dd", daysSinceCheck),
		daysSinceCheck:   daysSinceCheck,
	}
}

func portalCopy(count int) (string, string, string) {
	switch count {
	case 0:
		return "No portals", "need checking.", "No open applications are overdue for a portal check"
	case 1:
		return "One portal", "needs checking.", "1 needs a portal check"
	default:
		return fmt.Sprintf("%d portals", count), "need checking.", fmt.Sprintf("%d need a portal check", count)
	}
}

func organizationInitial(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "?"
	}
	return strings.ToUpper(string([]rune(name)[0]))
}

func organizationMark(organizationID int64) string {
	marks := []string{"mark-yellow", "mark-blue", "mark-orange", "mark-green"}
	return marks[(organizationID-1)%int64(len(marks))]
}

func formatSalary(minimum, maximum sql.NullInt64) string {
	if !minimum.Valid || !maximum.Valid {
		return "Not listed"
	}
	if minimum.Int64 == maximum.Int64 {
		return compactSalary(minimum.Int64)
	}
	return compactSalary(minimum.Int64) + "–" + compactSalary(maximum.Int64)
}

func compactSalary(amount int64) string {
	if amount%1000 == 0 {
		return fmt.Sprintf("$%dK", amount/1000)
	}
	return fmt.Sprintf("$%d", amount)
}

func formatStatus(status string) (string, string) {
	switch status {
	case "in_contact":
		return "In contact", "status-contact"
	case "accepted":
		return "Accepted", "status-accepted"
	case "rejected_after_contact":
		return "Rejected after contact", "status-rejected"
	case "rejected_no_contact":
		return "Rejected, no contact", "status-rejected"
	default:
		return "Applied", "status-applied"
	}
}

func formatLastChecked(lastChecked sql.NullString, now time.Time) (string, string, bool) {
	if !lastChecked.Valid {
		return "Not checked", "Check portal ↗", true
	}

	checkedAt, err := time.Parse(time.RFC3339Nano, lastChecked.String)
	if err != nil {
		return lastChecked.String, "", false
	}

	daysAgo := int(now.Sub(checkedAt).Hours() / 24)
	if daysAgo < 1 {
		return "Today", checkedAt.Format("3:04 PM"), false
	}
	if daysAgo == 1 {
		return "Yesterday", checkedAt.Format("3:04 PM"), false
	}
	return fmt.Sprintf("%d days ago", daysAgo), "Check portal ↗", daysAgo >= 7
}
