package main

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode/utf8"

	"github.com/pokemastercp/jobby/internal/database"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed db/migrations/*.sql web/templates/*.html web/static
var appFS embed.FS

var applicationLocation = mustLoadLocation("America/Chicago")

const heatmapWeeks = 16

type layoutData struct {
	Page                  string
	Settings              database.GetSettingsRow
	ReturnPath            string
	ApplicationCount      int
	OrganizationCount     int
	OpenCount             int
	DueCount              int
	OrganizationChoices   []organizationChoice
	ApplicationForm       applicationFormView
	EditApplicationForm   applicationFormView
	SelectedApplicationID int64
}

type organizationChoice struct {
	Name       string
	CareersURL string
}

type overviewData struct {
	layoutData
	Greeting                  string
	TodayLabel                string
	DueApplications           []portalApplicationView
	ConversationApplications  []applicationView
	NextCheck                 string
	CheckedTodayCount         int
	RoundTotal                int
	RoundPercent              int
	QuietDueCount             int
	Heatmap                   []heatmapWeekView
	HeatmapTotal              int
	AppliedCount              int
	InContactCount            int
	OfferCount                int
	RejectedAfterContactCount int
	RejectedNoContactCount    int
	WithdrawnCount            int
	ClosedCount               int
	ApplicationsLast7         int
	ApplicationsLast30        int
	ReachedContactCount       int
	ContactRate               int
	OfferRate                 int
	MedianOpenAge             string
	MedianResponseTime        string
	SalaryListedCount         int
	SalaryCoverage            int
	MedianOpenSalary          string
	RemoteCount               int
	LocalCount                int
	RemotePercent             int
	LocalPercent              int
}

type heatmapWeekView struct {
	Month string
	Days  []heatmapDayView
}

type heatmapDayView struct {
	Label    string
	Count    int
	Level    int
	IsToday  bool
	IsFuture bool
}

type applicationsPageData struct {
	layoutData
	Applications        []applicationView
	OrganizationOptions []organizationFilterOption
	StatusTabs          []statusTabView
	BoardColumns        []boardColumnView
	FilteredCount       int
	Filters             applicationFilters
	HasFilters          bool
	ClearURL            string
	TableURL            string
	BoardURL            string
}

type boardColumnView struct {
	Label        string
	Hint         string
	Status       string
	StatusClass  string
	Applications []applicationView
}

type statusTabView struct {
	Label  string
	Count  int
	URL    string
	Active bool
}

type organizationsPageData struct {
	layoutData
	Organizations          []organizationView
	OpenApplications       int64
	WithCareerPortal       int
	ActiveCount            int
	MissingPortalCount     int
	EditOrganizationForm   organizationFormView
	SelectedOrganizationID int64
}

type organizationView struct {
	ID                   int64
	Name                 string
	Initial              string
	Mark                 string
	CareersURL           string
	PortalLabel          string
	ApplicationCount     int64
	OpenApplicationCount int64
	NeedsPortal          bool
	LastApplied          string
	Applications         []organizationApplicationView
}

type organizationApplicationView struct {
	RoleTitle   string
	StatusLabel string
	StatusClass string
}

type applicationFilters struct {
	Status         string
	Income         string
	OrganizationID int64
	Sort           string
	Query          string
	View           string
}

type organizationFilterOption struct {
	ID   int64
	Name string
}

type applicationView struct {
	ID                  int64
	OrganizationID      int64
	OrganizationName    string
	OrganizationInitial string
	OrganizationMark    string
	RoleTitle           string
	Status              string
	Salary              string
	SalaryMin           string
	SalaryMax           string
	Location            string
	WorkLocation        string
	StatusLabel         string
	StatusClass         string
	CareersURL          string
	PostingURL          string
	AppliedAt           string
	AppliedAtDisplay    string
	AppliedAgo          string
	AppliedDays         int
	IsQuiet             bool
	AddedDisplay        string
	History             string
	StatusSince         string
	StatusDays          int
	LastChecked         string
	LastCheckedDetail   string
	LastCheckedIsDue    bool
	NeedsPortalCheck    bool
	Notes               string
}

type statusChangeView struct {
	Label string `json:"label"`
	Class string `json:"statusClass"`
	Date  string `json:"date"`
	Ago   string `json:"ago"`
}

type portalApplicationView struct {
	Application    applicationView
	daysSinceCheck int
}

type applicationFormView struct {
	HasErrors         bool
	GeneralError      string
	OrganizationName  string
	OrganizationError string
	RoleTitle         string
	RoleTitleError    string
	Status            string
	StatusError       string
	WorkLocation      string
	WorkLocationError string
	CareersURL        string
	CareersURLError   string
	PostingURL        string
	PostingURLError   string
	SalaryMin         string
	SalaryMinError    string
	SalaryMax         string
	SalaryMaxError    string
	AppliedAt         string
	AppliedAtError    string
	Notes             string
}

type applicationFormInput struct {
	OrganizationName string
	RoleTitle        string
	Status           string
	WorkLocation     string
	CareersURL       sql.NullString
	PostingURL       sql.NullString
	SalaryMin        sql.NullInt64
	SalaryMax        sql.NullInt64
	AppliedAt        sql.NullString
	Notes            sql.NullString
}

type overviewPageState struct {
	ApplicationForm       applicationFormView
	EditApplicationForm   applicationFormView
	SelectedApplicationID int64
}

type applicationsPageState struct {
	ApplicationForm       applicationFormView
	EditApplicationForm   applicationFormView
	SelectedApplicationID int64
}

type organizationsPageState struct {
	EditOrganizationForm   organizationFormView
	SelectedOrganizationID int64
}

type organizationFormView struct {
	Name            string
	NameError       string
	CareersURL      string
	CareersURLError string
	GeneralError    string
	HasErrors       bool
	Saved           bool
}

type organizationFormInput struct {
	Name       string
	CareersURL sql.NullString
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
	mux.HandleFunc("POST /settings", func(w http.ResponseWriter, r *http.Request) {
		if !isSameOrigin(r) {
			http.Error(w, "Invalid request origin", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Unable to read settings", http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(r.PostForm.Get("name"))
		days, err := strconv.ParseInt(r.PostForm.Get("portal_check_days"), 10, 64)
		quietDays, quietErr := strconv.ParseInt(r.PostForm.Get("quiet_after_days"), 10, 64)
		if !utf8.ValidString(name) || utf8.RuneCountInString(name) > 100 {
			http.Error(w, "Name must be 100 characters or fewer.", http.StatusUnprocessableEntity)
			return
		}
		if err != nil || days < 1 || days > 365 {
			http.Error(w, "Choose a portal check interval from 1 to 365 whole days.", http.StatusUnprocessableEntity)
			return
		}
		if quietErr != nil || quietDays < 1 || quietDays > 365 {
			http.Error(w, "Choose when to suggest closing, from 1 to 365 whole days.", http.StatusUnprocessableEntity)
			return
		}
		if err := queries.UpdateSettings(r.Context(), database.UpdateSettingsParams{Name: name, PortalCheckDays: days, QuietAfterDays: quietDays}); err != nil {
			log.Printf("save settings: %v", err)
			http.Error(w, "Unable to save settings. Please try again.", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		renderOverview(w, r, queries, templates, overviewPageState{}, http.StatusOK)
	})
	mux.HandleFunc("GET /applications", func(w http.ResponseWriter, r *http.Request) {
		renderApplications(w, r.Context(), queries, templates, parseApplicationFilters(r.URL.Query()), applicationsPageState{}, http.StatusOK)
	})
	mux.HandleFunc("GET /organizations", func(w http.ResponseWriter, r *http.Request) {
		state := organizationsPageState{}
		selectedID := r.URL.Query().Get("organization")
		if savedID := r.URL.Query().Get("saved"); savedID != "" {
			selectedID = savedID
			state.EditOrganizationForm.Saved = true
		}
		if organizationID, err := strconv.ParseInt(selectedID, 10, 64); err == nil && organizationID > 0 {
			state.SelectedOrganizationID = organizationID
		}
		renderOrganizations(w, r.Context(), queries, templates, state, http.StatusOK)
	})
	mux.HandleFunc("POST /organizations/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !isSameOrigin(r) {
			http.Error(w, "Invalid request origin", http.StatusForbidden)
			return
		}

		organizationID, err := parseRecordID(r)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		form, input := parseOrganizationForm(r)
		if form.HasErrors {
			renderOrganizations(w, r.Context(), queries, templates, organizationsPageState{
				EditOrganizationForm:   form,
				SelectedOrganizationID: organizationID,
			}, http.StatusUnprocessableEntity)
			return
		}

		nameInUse, err := queries.OrganizationNameInUse(r.Context(), database.OrganizationNameInUseParams{
			Name: input.Name,
			ID:   organizationID,
		})
		if err != nil {
			log.Printf("check organization name: %v", err)
			http.Error(w, "Unable to update organization", http.StatusInternalServerError)
			return
		}
		if nameInUse > 0 {
			form.NameError = "An organization with this name already exists."
			form.GeneralError = "Review the highlighted field and try again."
			form.HasErrors = true
			renderOrganizations(w, r.Context(), queries, templates, organizationsPageState{
				EditOrganizationForm:   form,
				SelectedOrganizationID: organizationID,
			}, http.StatusUnprocessableEntity)
			return
		}

		_, err = queries.UpdateOrganization(r.Context(), database.UpdateOrganizationParams{
			Name:       input.Name,
			CareersUrl: input.CareersURL,
			ID:         organizationID,
		})
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			log.Printf("update organization: %v", err)
			http.Error(w, "Unable to update organization", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, fmt.Sprintf("/organizations?saved=%d", organizationID), http.StatusSeeOther)
	})
	mux.HandleFunc("POST /applications", func(w http.ResponseWriter, r *http.Request) {
		if !isSameOrigin(r) {
			http.Error(w, "Invalid request origin", http.StatusForbidden)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		form, input := parseApplicationForm(r, false)
		if form.HasErrors {
			if referer, ok := applicationsReturnURL(r); ok {
				renderApplications(w, r.Context(), queries, templates, parseApplicationFilters(referer.Query()), applicationsPageState{ApplicationForm: form}, http.StatusUnprocessableEntity)
			} else {
				renderOverview(w, r, queries, templates, overviewPageState{ApplicationForm: form}, http.StatusUnprocessableEntity)
			}
			return
		}

		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			log.Printf("begin create application transaction: %v", err)
			http.Error(w, "Unable to save application", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		txQueries := queries.WithTx(tx)
		organization, err := txQueries.GetOrCreateOrganization(r.Context(), database.GetOrCreateOrganizationParams{
			Name:       input.OrganizationName,
			CareersUrl: input.CareersURL,
		})
		if err != nil {
			log.Printf("get or create organization: %v", err)
			http.Error(w, "Unable to save application", http.StatusInternalServerError)
			return
		}

		application, err := txQueries.CreateApplication(r.Context(), database.CreateApplicationParams{
			OrganizationID: organization.ID,
			RoleTitle:      input.RoleTitle,
			PostingUrl:     input.PostingURL,
			SalaryMin:      input.SalaryMin,
			SalaryMax:      input.SalaryMax,
			WorkLocation:   input.WorkLocation,
			AppliedAt:      input.AppliedAt,
			Notes:          input.Notes,
		})
		if err != nil {
			log.Printf("create application: %v", err)
			http.Error(w, "Unable to save application", http.StatusInternalServerError)
			return
		}
		if err := txQueries.CreateStatusChange(r.Context(), database.CreateStatusChangeParams{
			ApplicationID: application.ID,
			Status:        application.Status,
			ChangedAt:     application.CreatedAt,
		}); err != nil {
			log.Printf("record initial status: %v", err)
			http.Error(w, "Unable to save application", http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(); err != nil {
			log.Printf("commit create application: %v", err)
			http.Error(w, "Unable to save application", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, applicationMutationReturnPath(r), http.StatusSeeOther)
	})
	mux.HandleFunc("POST /applications/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !isSameOrigin(r) {
			http.Error(w, "Invalid request origin", http.StatusForbidden)
			return
		}

		applicationID, err := parseRecordID(r)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		form, input := parseApplicationForm(r, true)
		if form.HasErrors {
			if referer, ok := applicationsReturnURL(r); ok {
				renderApplications(w, r.Context(), queries, templates, parseApplicationFilters(referer.Query()), applicationsPageState{
					EditApplicationForm:   form,
					SelectedApplicationID: applicationID,
				}, http.StatusUnprocessableEntity)
			} else {
				renderOverview(w, r, queries, templates, overviewPageState{
					EditApplicationForm:   form,
					SelectedApplicationID: applicationID,
				}, http.StatusUnprocessableEntity)
			}
			return
		}

		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			log.Printf("begin update application transaction: %v", err)
			http.Error(w, "Unable to update application", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		txQueries := queries.WithTx(tx)
		organization, err := txQueries.GetOrCreateOrganization(r.Context(), database.GetOrCreateOrganizationParams{
			Name:       input.OrganizationName,
			CareersUrl: input.CareersURL,
		})
		if err != nil {
			log.Printf("get or create organization for application update: %v", err)
			http.Error(w, "Unable to update application", http.StatusInternalServerError)
			return
		}

		if err := txQueries.RecordStatusChange(r.Context(), database.RecordStatusChangeParams{
			Status: input.Status,
			ID:     applicationID,
		}); err != nil {
			log.Printf("record status change: %v", err)
			http.Error(w, "Unable to update application", http.StatusInternalServerError)
			return
		}

		_, err = txQueries.UpdateApplication(r.Context(), database.UpdateApplicationParams{
			OrganizationID: organization.ID,
			RoleTitle:      input.RoleTitle,
			Status:         input.Status,
			PostingUrl:     input.PostingURL,
			SalaryMin:      input.SalaryMin,
			SalaryMax:      input.SalaryMax,
			WorkLocation:   input.WorkLocation,
			AppliedAt:      input.AppliedAt,
			Notes:          input.Notes,
			ID:             applicationID,
		})
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			log.Printf("update application: %v", err)
			http.Error(w, "Unable to update application", http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(); err != nil {
			log.Printf("commit update application: %v", err)
			http.Error(w, "Unable to update application", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, applicationMutationReturnPath(r), http.StatusSeeOther)
	})
	mux.HandleFunc("POST /applications/{id}/status", func(w http.ResponseWriter, r *http.Request) {
		if !isSameOrigin(r) {
			http.Error(w, "Invalid request origin", http.StatusForbidden)
			return
		}

		applicationID, err := parseRecordID(r)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "The form could not be read. Please try again.", http.StatusUnprocessableEntity)
			return
		}
		status := strings.TrimSpace(r.PostForm.Get("status"))
		if !isApplicationStatus(status) {
			http.Error(w, "Choose a valid status.", http.StatusUnprocessableEntity)
			return
		}

		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			log.Printf("begin status transaction: %v", err)
			http.Error(w, "Unable to update application status", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		txQueries := queries.WithTx(tx)
		if err := txQueries.RecordStatusChange(r.Context(), database.RecordStatusChangeParams{
			Status: status,
			ID:     applicationID,
		}); err != nil {
			log.Printf("record status change: %v", err)
			http.Error(w, "Unable to update application status", http.StatusInternalServerError)
			return
		}
		rowsAffected, err := txQueries.UpdateApplicationStatus(r.Context(), database.UpdateApplicationStatusParams{
			Status: status,
			ID:     applicationID,
		})
		if err != nil {
			log.Printf("update application status: %v", err)
			http.Error(w, "Unable to update application status", http.StatusInternalServerError)
			return
		}
		if rowsAffected == 0 {
			http.NotFound(w, r)
			return
		}
		if err := tx.Commit(); err != nil {
			log.Printf("commit status change: %v", err)
			http.Error(w, "Unable to update application status", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, applicationMutationReturnPath(r), http.StatusSeeOther)
	})
	mux.HandleFunc("POST /applications/{id}/checked", func(w http.ResponseWriter, r *http.Request) {
		if !isSameOrigin(r) {
			http.Error(w, "Invalid request origin", http.StatusForbidden)
			return
		}

		applicationID, err := parseRecordID(r)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		if _, err := queries.MarkApplicationChecked(r.Context(), applicationID); err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		} else if err != nil {
			log.Printf("mark application checked: %v", err)
			http.Error(w, "Unable to mark application checked", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, applicationMutationReturnPath(r), http.StatusSeeOther)
	})
	mux.HandleFunc("POST /applications/{id}/delete", func(w http.ResponseWriter, r *http.Request) {
		if !isSameOrigin(r) {
			http.Error(w, "Invalid request origin", http.StatusForbidden)
			return
		}

		applicationID, err := parseRecordID(r)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		if _, err := queries.DeleteApplication(r.Context(), applicationID); err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		} else if err != nil {
			log.Printf("delete application: %v", err)
			http.Error(w, "Unable to delete application", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, applicationMutationReturnPath(r), http.StatusSeeOther)
	})

	addr := ":80"
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func renderOverview(w http.ResponseWriter, r *http.Request, queries *database.Queries, templates *template.Template, state overviewPageState, status int) {
	data, err := loadOverviewData(r.Context(), queries)
	if err != nil {
		log.Printf("list applications: %v", err)
		http.Error(w, "Unable to load applications", http.StatusInternalServerError)
		return
	}
	data.ApplicationForm = state.ApplicationForm
	data.EditApplicationForm = state.EditApplicationForm
	data.SelectedApplicationID = state.SelectedApplicationID

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := templates.ExecuteTemplate(w, "index.html", data); err != nil {
		log.Printf("render overview: %v", err)
	}
}

func renderApplications(w http.ResponseWriter, ctx context.Context, queries *database.Queries, templates *template.Template, filters applicationFilters, state applicationsPageState, status int) {
	data, err := loadApplicationsPageData(ctx, queries, filters)
	if err != nil {
		log.Printf("list applications page: %v", err)
		http.Error(w, "Unable to load applications", http.StatusInternalServerError)
		return
	}
	data.ApplicationForm = state.ApplicationForm
	data.EditApplicationForm = state.EditApplicationForm
	data.SelectedApplicationID = state.SelectedApplicationID

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := templates.ExecuteTemplate(w, "applications.html", data); err != nil {
		log.Printf("render applications page: %v", err)
	}
}

func renderOrganizations(w http.ResponseWriter, ctx context.Context, queries *database.Queries, templates *template.Template, state organizationsPageState, status int) {
	data, err := loadOrganizationsPageData(ctx, queries, state)
	if err != nil {
		log.Printf("list organizations page: %v", err)
		http.Error(w, "Unable to load organizations", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := templates.ExecuteTemplate(w, "organizations.html", data); err != nil {
		log.Printf("render organizations page: %v", err)
	}
}

func loadOverviewData(ctx context.Context, queries *database.Queries) (overviewData, error) {
	settings, err := queries.GetSettings(ctx)
	if err != nil {
		return overviewData{}, err
	}
	applications, err := queries.ListApplications(ctx)
	if err != nil {
		return overviewData{}, err
	}
	organizations, err := queries.ListOrganizations(ctx)
	if err != nil {
		return overviewData{}, err
	}
	history, err := loadStatusHistory(ctx, queries)
	if err != nil {
		return overviewData{}, err
	}

	now := time.Now().In(applicationLocation)
	data := overviewData{
		layoutData:         newLayoutData("dashboard", settings, applications, organizations, now),
		Greeting:           greeting(now, settings.Name),
		TodayLabel:         now.Format("Monday, January 2"),
		MedianOpenAge:      "—",
		MedianResponseTime: "—",
		MedianOpenSalary:   "—",
	}
	data.ReturnPath = "/"

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, applicationLocation)
	weekStart := today.AddDate(0, 0, -(int(today.Weekday())+6)%7)
	heatmapStart := weekStart.AddDate(0, 0, -7*(heatmapWeeks-1))
	dayCounts := make([]int, heatmapWeeks*7)
	openAges := make([]int, 0, len(applications))
	responseTimes := make([]int, 0, len(applications))
	openSalaryMidpoints := make([]int64, 0, len(applications))
	conversations := make([]database.ListApplicationsRow, 0)
	nextCheckDays, nextCheckName := -1, ""

	for _, application := range applications {
		activityDate, hasActivityDate := applicationActivityDate(application)
		if hasActivityDate {
			activityDay := time.Date(activityDate.Year(), activityDate.Month(), activityDate.Day(), 0, 0, 0, 0, applicationLocation)
			if !activityDay.After(today) {
				if !activityDay.Before(today.AddDate(0, 0, -6)) {
					data.ApplicationsLast7++
				}
				if !activityDay.Before(today.AddDate(0, 0, -29)) {
					data.ApplicationsLast30++
				}
				if dayIndex := calendarDaysBetween(heatmapStart, activityDay); dayIndex >= 0 && dayIndex < len(dayCounts) {
					dayCounts[dayIndex]++
					data.HeatmapTotal++
				}
			}
		}

		if hasActivityDate {
			if repliedAt, ok := firstReply(history[application.ID]); ok && !repliedAt.Before(activityDate) {
				responseTimes = append(responseTimes, calendarDaysBetween(activityDate, repliedAt.In(applicationLocation)))
			}
		}

		if application.SalaryMin.Valid && application.SalaryMax.Valid {
			data.SalaryListedCount++
		}
		if application.WorkLocation == "remote" {
			data.RemoteCount++
		} else {
			data.LocalCount++
		}

		switch application.Status {
		case "applied":
			data.AppliedCount++
		case "in_contact":
			data.InContactCount++
			conversations = append(conversations, application)
		case "offer":
			data.OfferCount++
			conversations = append(conversations, application)
		case "rejected_after_contact":
			data.RejectedAfterContactCount++
		case "rejected_no_contact":
			data.RejectedNoContactCount++
		case "withdrawn":
			data.WithdrawnCount++
		}

		if !isOpenStatus(application.Status) {
			continue
		}

		if hasActivityDate && !activityDate.After(now) {
			openAges = append(openAges, calendarDaysBetween(activityDate, now))
		}
		if application.SalaryMin.Valid && application.SalaryMax.Valid {
			openSalaryMidpoints = append(openSalaryMidpoints, application.SalaryMin.Int64+(application.SalaryMax.Int64-application.SalaryMin.Int64)/2)
		}
		view := newApplicationView(application, history[application.ID], now, settings)
		if !view.NeedsPortalCheck {
			continue
		}
		if view.LastCheckedIsDue {
			data.DueApplications = append(data.DueApplications, newPortalApplicationView(application, view, now))
			if view.IsQuiet {
				data.QuietDueCount++
			}
		} else if checkedAt, err := time.Parse(time.RFC3339Nano, application.LastCheckedAt.String); err == nil {
			daysSinceCheck := calendarDaysBetween(checkedAt.In(now.Location()), now)
			// Creating an application records a check at the same instant; that isn't part of today's round.
			if daysSinceCheck == 0 && application.LastCheckedAt.String != application.CreatedAt {
				data.CheckedTodayCount++
			}
			remaining := int(settings.PortalCheckDays) - daysSinceCheck
			if nextCheckDays < 0 || remaining < nextCheckDays {
				nextCheckDays, nextCheckName = remaining, application.OrganizationName
			}
		}
	}

	data.ClosedCount = data.RejectedAfterContactCount + data.RejectedNoContactCount + data.WithdrawnCount
	data.ReachedContactCount = data.InContactCount + data.OfferCount + data.RejectedAfterContactCount
	data.ContactRate = roundedPercentage(data.ReachedContactCount, data.ApplicationCount)
	// Withdrawn applications never got a decision, so they don't count toward the offer rate.
	data.OfferRate = roundedPercentage(data.OfferCount, data.OfferCount+data.RejectedAfterContactCount+data.RejectedNoContactCount)
	data.SalaryCoverage = roundedPercentage(data.SalaryListedCount, data.ApplicationCount)
	data.RemotePercent = roundedPercentage(data.RemoteCount, data.ApplicationCount)
	data.LocalPercent = roundedPercentage(data.LocalCount, data.ApplicationCount)
	data.RoundTotal = len(data.DueApplications) + data.CheckedTodayCount
	data.RoundPercent = roundedPercentage(data.CheckedTodayCount, data.RoundTotal)

	if len(openAges) > 0 {
		sort.Ints(openAges)
		data.MedianOpenAge = fmt.Sprintf("%dd", medianInt(openAges))
	}
	if len(responseTimes) > 0 {
		sort.Ints(responseTimes)
		data.MedianResponseTime = fmt.Sprintf("%dd", medianInt(responseTimes))
	}
	if len(openSalaryMidpoints) > 0 {
		sort.Slice(openSalaryMidpoints, func(i, j int) bool {
			return openSalaryMidpoints[i] < openSalaryMidpoints[j]
		})
		data.MedianOpenSalary = compactSalary(medianInt64(openSalaryMidpoints))
	}

	for week := range heatmapWeeks {
		start := heatmapStart.AddDate(0, 0, week*7)
		view := heatmapWeekView{Days: make([]heatmapDayView, 7)}
		if week > 0 && start.Month() != start.AddDate(0, 0, -7).Month() {
			view.Month = start.Format("Jan")
		}
		for weekday := range 7 {
			day := start.AddDate(0, 0, weekday)
			count := dayCounts[week*7+weekday]
			noun := "applications"
			if count == 1 {
				noun = "application"
			}
			view.Days[weekday] = heatmapDayView{
				Label:    fmt.Sprintf("%s: %d %s", day.Format("Mon, Jan 2"), count, noun),
				Count:    count,
				Level:    heatLevel(count),
				IsToday:  day.Equal(today),
				IsFuture: day.After(today),
			}
		}
		data.Heatmap = append(data.Heatmap, view)
	}

	sort.Slice(data.DueApplications, func(i, j int) bool {
		return data.DueApplications[i].daysSinceCheck > data.DueApplications[j].daysSinceCheck
	})

	// Offers come first; after that, conversations that have gone quiet the longest are the likeliest to need a follow-up.
	sort.SliceStable(conversations, func(i, j int) bool {
		firstIsOffer, secondIsOffer := conversations[i].Status == "offer", conversations[j].Status == "offer"
		if firstIsOffer != secondIsOffer {
			return firstIsOffer
		}
		return conversations[i].StatusChangedAt < conversations[j].StatusChangedAt
	})
	for _, application := range conversations {
		data.ConversationApplications = append(data.ConversationApplications, newApplicationView(application, history[application.ID], now, settings))
	}

	switch {
	case nextCheckDays == 1:
		data.NextCheck = fmt.Sprintf("Next up: %s tomorrow.", nextCheckName)
	case nextCheckDays > 1:
		data.NextCheck = fmt.Sprintf("Next up: %s in %d days.", nextCheckName, nextCheckDays)
	}

	return data, nil
}

func loadStatusHistory(ctx context.Context, queries *database.Queries) (map[int64][]database.ListStatusChangesRow, error) {
	changes, err := queries.ListStatusChanges(ctx)
	if err != nil {
		return nil, err
	}
	history := make(map[int64][]database.ListStatusChangesRow)
	for _, change := range changes {
		history[change.ApplicationID] = append(history[change.ApplicationID], change)
	}
	return history, nil
}

// A reply is the first move into a status that means the organization got in touch.
func firstReply(changes []database.ListStatusChangesRow) (time.Time, bool) {
	for _, change := range changes {
		switch change.Status {
		case "in_contact", "offer", "rejected_after_contact":
			changedAt, err := time.Parse(time.RFC3339Nano, change.ChangedAt)
			return changedAt, err == nil
		}
	}
	return time.Time{}, false
}

func heatLevel(count int) int {
	switch {
	case count >= 5:
		return 4
	case count >= 3:
		return 3
	default:
		return count
	}
}

func newLayoutData(page string, settings database.GetSettingsRow, applications []database.ListApplicationsRow, organizations []database.ListOrganizationsRow, now time.Time) layoutData {
	data := layoutData{
		Page:              page,
		Settings:          settings,
		ApplicationCount:  len(applications),
		OrganizationCount: len(organizations),
	}
	for _, organization := range organizations {
		data.OrganizationChoices = append(data.OrganizationChoices, organizationChoice{
			Name:       organization.Name,
			CareersURL: nullStringValue(organization.CareersUrl),
		})
	}
	for _, application := range applications {
		if isOpenStatus(application.Status) {
			data.OpenCount++
		}
		if !needsPortalCheck(application.Status) {
			continue
		}
		if _, _, isDue := formatLastChecked(application.LastCheckedAt, now, int(settings.PortalCheckDays)); isDue {
			data.DueCount++
		}
	}
	return data
}

func greeting(now time.Time, name string) string {
	greeting := "Good evening"
	if now.Hour() < 12 {
		greeting = "Good morning"
	} else if now.Hour() < 17 {
		greeting = "Good afternoon"
	}
	if name != "" {
		return greeting + ", " + name
	}
	return greeting
}

func applicationActivityDate(application database.ListApplicationsRow) (time.Time, bool) {
	if application.AppliedAt.Valid {
		if appliedAt, err := time.ParseInLocation("2006-01-02", application.AppliedAt.String, applicationLocation); err == nil {
			return appliedAt, true
		}
	}

	createdAt, err := time.Parse(time.RFC3339Nano, application.CreatedAt)
	if err != nil {
		return time.Time{}, false
	}
	return createdAt.In(applicationLocation), true
}

func roundedPercentage(part int, total int) int {
	if total == 0 {
		return 0
	}
	return (part*100 + total/2) / total
}

func medianInt(values []int) int {
	middle := len(values) / 2
	if len(values)%2 == 1 {
		return values[middle]
	}
	return values[middle-1] + (values[middle]-values[middle-1])/2
}

func medianInt64(values []int64) int64 {
	middle := len(values) / 2
	if len(values)%2 == 1 {
		return values[middle]
	}
	return values[middle-1] + (values[middle]-values[middle-1])/2
}

func loadApplicationsPageData(ctx context.Context, queries *database.Queries, filters applicationFilters) (applicationsPageData, error) {
	settings, err := queries.GetSettings(ctx)
	if err != nil {
		return applicationsPageData{}, err
	}
	applications, err := queries.ListApplications(ctx)
	if err != nil {
		return applicationsPageData{}, err
	}
	organizations, err := queries.ListOrganizations(ctx)
	if err != nil {
		return applicationsPageData{}, err
	}
	history, err := loadStatusHistory(ctx, queries)
	if err != nil {
		return applicationsPageData{}, err
	}

	now := time.Now().In(applicationLocation)
	data := applicationsPageData{
		layoutData: newLayoutData("applications", settings, applications, organizations, now),
		Filters:    filters,
		HasFilters: filters.Income != "" || filters.OrganizationID != 0 || filters.Query != "",
		ClearURL:   applicationsURL(applicationFilters{Status: filters.Status, View: filters.View}),
	}
	data.ReturnPath = applicationsURL(filters)
	tableFilters, boardFilters := filters, filters
	tableFilters.View = ""
	boardFilters.View, boardFilters.Status = "board", ""
	data.TableURL, data.BoardURL = applicationsURL(tableFilters), applicationsURL(boardFilters)
	for _, organization := range organizations {
		data.OrganizationOptions = append(data.OrganizationOptions, organizationFilterOption{
			ID:   organization.ID,
			Name: organization.Name,
		})
	}

	// Tab counts ignore the text search so they stay stable while the list is filtered in the browser.
	tabs := []struct{ status, label string }{
		{"", "All"},
		{"open", "Open"},
		{"applied", "Applied"},
		{"in_contact", "Interviewing"},
		{"offer", "Offer"},
		{"closed", "Closed"},
	}
	for _, tab := range tabs {
		tabFilters := filters
		tabFilters.Status = tab.status
		countFilters := tabFilters
		countFilters.Query = ""
		count := 0
		for _, application := range applications {
			if applicationMatchesFilters(application, countFilters) {
				count++
			}
		}
		data.StatusTabs = append(data.StatusTabs, statusTabView{
			Label:  tab.label,
			Count:  count,
			URL:    applicationsURL(tabFilters),
			Active: filters.Status == tab.status,
		})
	}

	matched := make([]database.ListApplicationsRow, 0, len(applications))
	for _, application := range applications {
		if applicationMatchesFilters(application, filters) {
			matched = append(matched, application)
		}
	}
	sortApplications(matched, filters.Sort)
	for _, application := range matched {
		data.Applications = append(data.Applications, newApplicationView(application, history[application.ID], now, settings))
	}
	data.FilteredCount = len(data.Applications)

	if filters.View == "board" {
		data.BoardColumns = []boardColumnView{
			{Label: "Applied", Hint: "Waiting to hear back", Status: "applied", StatusClass: "status-applied"},
			{Label: "Interviewing", Hint: "Talking with the team", Status: "in_contact", StatusClass: "status-contact"},
			{Label: "Offer", Hint: "Waiting on your decision", Status: "offer", StatusClass: "status-offer"},
			{Label: "Closed", Hint: "Rejected, no response, or withdrawn", StatusClass: "status-closed"},
		}
		for _, application := range data.Applications {
			column := 3
			switch application.Status {
			case "applied":
				column = 0
			case "in_contact":
				column = 1
			case "offer":
				column = 2
			}
			data.BoardColumns[column].Applications = append(data.BoardColumns[column].Applications, application)
		}
	}

	return data, nil
}

func applicationsURL(filters applicationFilters) string {
	query := url.Values{}
	if filters.Status != "" {
		query.Set("status", filters.Status)
	}
	if filters.Income != "" {
		query.Set("income", filters.Income)
	}
	if filters.OrganizationID != 0 {
		query.Set("organization", strconv.FormatInt(filters.OrganizationID, 10))
	}
	if filters.Sort != "" {
		query.Set("sort", filters.Sort)
	}
	if filters.Query != "" {
		query.Set("q", filters.Query)
	}
	if filters.View != "" {
		query.Set("view", filters.View)
	}
	return (&url.URL{Path: "/applications", RawQuery: query.Encode()}).RequestURI()
}

func sortApplications(applications []database.ListApplicationsRow, order string) {
	switch order {
	case "applied":
		sort.SliceStable(applications, func(i, j int) bool {
			first, _ := applicationActivityDate(applications[i])
			second, _ := applicationActivityDate(applications[j])
			return first.After(second)
		})
	case "salary":
		sort.SliceStable(applications, func(i, j int) bool {
			return applications[i].SalaryMax.Int64 > applications[j].SalaryMax.Int64
		})
	case "checked":
		sort.SliceStable(applications, func(i, j int) bool {
			first, second := applications[i].LastCheckedAt, applications[j].LastCheckedAt
			if first.Valid != second.Valid {
				return !first.Valid
			}
			return first.String < second.String
		})
	}
}

func loadOrganizationsPageData(ctx context.Context, queries *database.Queries, state organizationsPageState) (organizationsPageData, error) {
	settings, err := queries.GetSettings(ctx)
	if err != nil {
		return organizationsPageData{}, err
	}
	applications, err := queries.ListApplications(ctx)
	if err != nil {
		return organizationsPageData{}, err
	}
	organizations, err := queries.ListOrganizations(ctx)
	if err != nil {
		return organizationsPageData{}, err
	}

	now := time.Now().In(applicationLocation)
	data := organizationsPageData{
		layoutData:             newLayoutData("organizations", settings, applications, organizations, now),
		EditOrganizationForm:   state.EditOrganizationForm,
		SelectedOrganizationID: state.SelectedOrganizationID,
	}
	data.ReturnPath = "/organizations"
	applicationsByOrganization := make(map[int64][]database.ListApplicationsRow)
	for _, application := range applications {
		applicationsByOrganization[application.OrganizationID] = append(applicationsByOrganization[application.OrganizationID], application)
	}
	foundSelection := false
	for _, organization := range organizations {
		view := newOrganizationView(organization, applicationsByOrganization[organization.ID], now)
		data.Organizations = append(data.Organizations, view)
		data.OpenApplications += organization.OpenApplicationCount
		if view.CareersURL != "" {
			data.WithCareerPortal++
		}
		if organization.OpenApplicationCount > 0 {
			data.ActiveCount++
		}
		if view.NeedsPortal {
			data.MissingPortalCount++
		}
		if organization.ID == state.SelectedOrganizationID {
			foundSelection = true
			if !state.EditOrganizationForm.HasErrors {
				data.EditOrganizationForm.Name = organization.Name
				data.EditOrganizationForm.CareersURL = view.CareersURL
			}
		}
	}
	if !foundSelection {
		data.SelectedOrganizationID = 0
	}
	return data, nil
}

func parseApplicationFilters(values url.Values) applicationFilters {
	filters := applicationFilters{}
	status := strings.TrimSpace(values.Get("status"))
	if status == "open" || status == "closed" || isApplicationStatus(status) {
		filters.Status = status
	}

	income := strings.TrimSpace(values.Get("income"))
	switch income {
	case "listed", "75000", "100000", "125000", "150000":
		filters.Income = income
	}

	organizationID, err := strconv.ParseInt(values.Get("organization"), 10, 64)
	if err == nil && organizationID > 0 {
		filters.OrganizationID = organizationID
	}

	switch sortOrder := values.Get("sort"); sortOrder {
	case "applied", "salary", "checked":
		filters.Sort = sortOrder
	}

	query := normalizeSingleLine(values.Get("q"))
	if utf8.ValidString(query) && utf8.RuneCountInString(query) <= 100 {
		filters.Query = query
	}

	// The board groups every status into columns, so it ignores the status filter.
	if values.Get("view") == "board" {
		filters.View = "board"
		filters.Status = ""
	}
	return filters
}

func applicationMatchesFilters(application database.ListApplicationsRow, filters applicationFilters) bool {
	switch filters.Status {
	case "":
	case "open":
		if !isOpenStatus(application.Status) {
			return false
		}
	case "closed":
		if isOpenStatus(application.Status) {
			return false
		}
	default:
		if application.Status != filters.Status {
			return false
		}
	}
	if filters.OrganizationID != 0 && application.OrganizationID != filters.OrganizationID {
		return false
	}
	if filters.Income == "listed" && (!application.SalaryMin.Valid || !application.SalaryMax.Valid) {
		return false
	}
	if filters.Income != "" && filters.Income != "listed" {
		minimum, _ := strconv.ParseInt(filters.Income, 10, 64)
		if !application.SalaryMax.Valid || application.SalaryMax.Int64 < minimum {
			return false
		}
	}
	if filters.Query != "" {
		haystack := strings.ToLower(application.OrganizationName + " " + application.RoleTitle + " " + application.Notes.String)
		for _, term := range strings.Fields(strings.ToLower(filters.Query)) {
			if !strings.Contains(haystack, term) {
				return false
			}
		}
	}
	return true
}

func applicationsReturnURL(r *http.Request) (*url.URL, bool) {
	source := r.PostForm.Get("return_to")
	if source == "" {
		source = r.Referer()
	}
	referer, err := url.Parse(source)
	if err != nil || referer.Path != "/applications" {
		return nil, false
	}
	return referer, true
}

func applicationMutationReturnPath(r *http.Request) string {
	if referer, ok := applicationsReturnURL(r); ok {
		return referer.RequestURI()
	}
	return "/"
}

func parseApplicationForm(r *http.Request, includeStatus bool) (applicationFormView, applicationFormInput) {
	form := applicationFormView{}
	if err := r.ParseForm(); err != nil {
		form.HasErrors = true
		form.GeneralError = "The form could not be read. Please try again."
		return form, applicationFormInput{}
	}

	form.OrganizationName = normalizeSingleLine(r.FormValue("organization_name"))
	form.RoleTitle = normalizeSingleLine(r.FormValue("role_title"))
	if includeStatus {
		form.Status = strings.TrimSpace(r.FormValue("status"))
	}
	form.WorkLocation = strings.TrimSpace(r.FormValue("work_location"))
	if !includeStatus {
		form.CareersURL = strings.TrimSpace(r.FormValue("careers_url"))
	}
	form.PostingURL = strings.TrimSpace(r.FormValue("posting_url"))
	form.SalaryMin = strings.TrimSpace(r.FormValue("salary_min"))
	form.SalaryMax = strings.TrimSpace(r.FormValue("salary_max"))
	form.AppliedAt = strings.TrimSpace(r.FormValue("applied_at"))
	form.Notes = strings.TrimSpace(r.FormValue("notes"))

	if form.OrganizationName == "" {
		form.OrganizationError = "Enter an organization."
	}
	if form.RoleTitle == "" {
		form.RoleTitleError = "Enter a role title."
	}
	if includeStatus && !isApplicationStatus(form.Status) {
		form.StatusError = "Choose a valid status."
	}
	if form.WorkLocation != "remote" && form.WorkLocation != "local" {
		form.WorkLocationError = "Choose remote or local."
	}

	if !includeStatus {
		form.CareersURLError = validateOptionalHTTPURL(form.CareersURL)
	}
	form.PostingURLError = validateOptionalHTTPURL(form.PostingURL)

	salaryMin, salaryMinError := parseOptionalSalary(form.SalaryMin)
	salaryMax, salaryMaxError := parseOptionalSalary(form.SalaryMax)
	form.SalaryMinError = salaryMinError
	form.SalaryMaxError = salaryMaxError
	if salaryMin.Valid != salaryMax.Valid {
		if !salaryMin.Valid && form.SalaryMinError == "" {
			form.SalaryMinError = "Enter both ends of the salary range."
		}
		if !salaryMax.Valid && form.SalaryMaxError == "" {
			form.SalaryMaxError = "Enter both ends of the salary range."
		}
	}
	if salaryMin.Valid && salaryMax.Valid && salaryMax.Int64 < salaryMin.Int64 {
		form.SalaryMaxError = "Maximum salary must be at least the minimum."
	}

	if form.AppliedAt != "" {
		if _, err := time.Parse("2006-01-02", form.AppliedAt); err != nil {
			form.AppliedAtError = "Enter a valid date."
		}
	}

	form.HasErrors = form.OrganizationError != "" ||
		form.RoleTitleError != "" ||
		form.StatusError != "" ||
		form.WorkLocationError != "" ||
		form.CareersURLError != "" ||
		form.PostingURLError != "" ||
		form.SalaryMinError != "" ||
		form.SalaryMaxError != "" ||
		form.AppliedAtError != ""
	if form.HasErrors {
		form.GeneralError = "Review the highlighted fields and try again."
		return form, applicationFormInput{}
	}

	return form, applicationFormInput{
		OrganizationName: form.OrganizationName,
		RoleTitle:        form.RoleTitle,
		Status:           form.Status,
		WorkLocation:     form.WorkLocation,
		CareersURL:       nullableString(form.CareersURL),
		PostingURL:       nullableString(form.PostingURL),
		SalaryMin:        salaryMin,
		SalaryMax:        salaryMax,
		AppliedAt:        nullableString(form.AppliedAt),
		Notes:            nullableString(form.Notes),
	}
}

func newOrganizationView(organization database.ListOrganizationsRow, applications []database.ListApplicationsRow, now time.Time) organizationView {
	careersURL := nullStringValue(organization.CareersUrl)
	portalLabel := "Not saved"
	if careersURL != "" {
		portalLabel = "Portal saved"
		if parsedURL, err := url.Parse(careersURL); err == nil && parsedURL.Hostname() != "" {
			portalLabel = strings.TrimPrefix(parsedURL.Hostname(), "www.")
		}
	}

	view := organizationView{
		ID:                   organization.ID,
		Name:                 organization.Name,
		Initial:              organizationInitial(organization.Name),
		Mark:                 organizationMark(organization.ID),
		CareersURL:           careersURL,
		PortalLabel:          portalLabel,
		ApplicationCount:     organization.ApplicationCount,
		OpenApplicationCount: organization.OpenApplicationCount,
		NeedsPortal:          careersURL == "" && organization.OpenApplicationCount > 0,
	}

	var latest time.Time
	for _, application := range applications {
		label, class := formatStatus(application.Status)
		view.Applications = append(view.Applications, organizationApplicationView{
			RoleTitle:   application.RoleTitle,
			StatusLabel: label,
			StatusClass: class,
		})
		if activityDate, ok := applicationActivityDate(application); ok && activityDate.After(latest) {
			latest = activityDate
		}
	}
	if !latest.IsZero() {
		view.LastApplied = "Last applied " + strings.ToLower(relativeDays(calendarDaysBetween(latest, now)))
	}
	return view
}

func parseOrganizationForm(r *http.Request) (organizationFormView, organizationFormInput) {
	form := organizationFormView{}
	if err := r.ParseForm(); err != nil {
		form.HasErrors = true
		form.GeneralError = "The form could not be read. Please try again."
		return form, organizationFormInput{}
	}

	form.Name = normalizeSingleLine(r.FormValue("name"))
	form.CareersURL = strings.TrimSpace(r.FormValue("careers_url"))
	if form.Name == "" {
		form.NameError = "Enter an organization name."
	}
	form.CareersURLError = validateOptionalHTTPURL(form.CareersURL)
	form.HasErrors = form.NameError != "" || form.CareersURLError != ""
	if form.HasErrors {
		form.GeneralError = "Review the highlighted fields and try again."
		return form, organizationFormInput{}
	}

	return form, organizationFormInput{
		Name:       form.Name,
		CareersURL: nullableString(form.CareersURL),
	}
}

func validateOptionalHTTPURL(value string) string {
	if value == "" {
		return ""
	}

	parsedURL, err := url.ParseRequestURI(value)
	if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return "Enter a complete HTTP or HTTPS URL."
	}
	return ""
}

func normalizeSingleLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func parseOptionalSalary(value string) (sql.NullInt64, string) {
	if value == "" {
		return sql.NullInt64{}, ""
	}

	amount, err := strconv.ParseInt(value, 10, 64)
	if err != nil || amount < 0 {
		return sql.NullInt64{}, "Enter a non-negative whole number."
	}
	return sql.NullInt64{Int64: amount, Valid: true}, ""
}

func nullableString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

func mustLoadLocation(name string) *time.Location {
	location, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return location
}

func parseRecordID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, fmt.Errorf("invalid record id")
	}
	return id, nil
}

func isSameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	return origin == "" || origin == "http://"+r.Host || origin == "https://"+r.Host
}

func isOpenStatus(status string) bool {
	return status == "applied" || status == "in_contact" || status == "offer"
}

// Portal checks look for news on an application, which stops mattering once an offer arrives.
func needsPortalCheck(status string) bool {
	return status == "applied" || status == "in_contact"
}

func isApplicationStatus(status string) bool {
	switch status {
	case "applied", "in_contact", "offer", "rejected_after_contact", "rejected_no_contact", "withdrawn":
		return true
	default:
		return false
	}
}

func newApplicationView(application database.ListApplicationsRow, changes []database.ListStatusChangesRow, now time.Time, settings database.GetSettingsRow) applicationView {
	lastChecked, lastCheckedDetail, lastCheckedIsDue := formatLastChecked(application.LastCheckedAt, now, int(settings.PortalCheckDays))
	statusLabel, statusClass := formatStatus(application.Status)

	statusSince, statusDays := "", 0
	if changedAt, err := time.Parse(time.RFC3339Nano, application.StatusChangedAt); err == nil {
		statusDays = calendarDaysBetween(changedAt.In(now.Location()), now)
		statusSince = relativeDays(statusDays)
	}
	appliedAgo, appliedDays := "", 0
	if activityDate, ok := applicationActivityDate(application); ok {
		appliedDays = calendarDaysBetween(activityDate, now)
		appliedAgo = relativeDays(appliedDays)
	}

	return applicationView{
		ID:                  application.ID,
		OrganizationID:      application.OrganizationID,
		OrganizationName:    application.OrganizationName,
		OrganizationInitial: organizationInitial(application.OrganizationName),
		OrganizationMark:    organizationMark(application.OrganizationID),
		RoleTitle:           application.RoleTitle,
		Status:              application.Status,
		Salary:              formatSalary(application.SalaryMin, application.SalaryMax),
		SalaryMin:           formatOptionalInt(application.SalaryMin),
		SalaryMax:           formatOptionalInt(application.SalaryMax),
		Location:            strings.ToUpper(application.WorkLocation[:1]) + application.WorkLocation[1:],
		WorkLocation:        application.WorkLocation,
		StatusLabel:         statusLabel,
		StatusClass:         statusClass,
		CareersURL:          nullStringValue(application.OrganizationCareersUrl),
		PostingURL:          nullStringValue(application.PostingUrl),
		AppliedAt:           nullStringValue(application.AppliedAt),
		AppliedAtDisplay:    formatAppliedAt(application.AppliedAt),
		AppliedAgo:          appliedAgo,
		AppliedDays:         appliedDays,
		IsQuiet:             application.Status == "applied" && appliedDays >= int(settings.QuietAfterDays),
		AddedDisplay:        formatTimestampDate(application.CreatedAt),
		History:             formatHistory(changes, now),
		StatusSince:         statusSince,
		StatusDays:          statusDays,
		LastChecked:         lastChecked,
		LastCheckedDetail:   lastCheckedDetail,
		LastCheckedIsDue:    lastCheckedIsDue && needsPortalCheck(application.Status),
		NeedsPortalCheck:    needsPortalCheck(application.Status),
		Notes:               nullStringValue(application.Notes),
	}
}

func newPortalApplicationView(application database.ListApplicationsRow, view applicationView, now time.Time) portalApplicationView {
	if !application.LastCheckedAt.Valid {
		return portalApplicationView{
			Application:    view,
			daysSinceCheck: int(^uint(0) >> 1),
		}
	}

	checkedAt, _ := time.Parse(time.RFC3339Nano, application.LastCheckedAt.String)
	daysSinceCheck := calendarDaysBetween(checkedAt.In(now.Location()), now)
	return portalApplicationView{
		Application:    view,
		daysSinceCheck: daysSinceCheck,
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
	return fmt.Sprintf("mark-%d", (organizationID-1)%6+1)
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
	if amount < 1000 {
		return fmt.Sprintf("$%d", amount)
	}

	wholeThousands := amount / 1000
	remainder := amount % 1000
	if remainder == 0 {
		return fmt.Sprintf("$%dK", wholeThousands)
	}

	decimal := strings.TrimRight(fmt.Sprintf("%03d", remainder), "0")
	return fmt.Sprintf("$%d.%sK", wholeThousands, decimal)
}

func formatOptionalInt(value sql.NullInt64) string {
	if !value.Valid {
		return ""
	}
	return strconv.FormatInt(value.Int64, 10)
}

func nullStringValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func formatAppliedAt(appliedAt sql.NullString) string {
	if !appliedAt.Valid {
		return "Not recorded"
	}
	appliedDate, err := time.Parse("2006-01-02", appliedAt.String)
	if err != nil {
		return appliedAt.String
	}
	return appliedDate.Format("Jan 2, 2006")
}

// The first entry records the application being added as Applied, which the timeline already shows.
func formatHistory(changes []database.ListStatusChangesRow, now time.Time) string {
	views := make([]statusChangeView, 0, len(changes))
	for index, change := range changes {
		if index == 0 && change.Status == "applied" {
			continue
		}
		label, class := formatStatus(change.Status)
		view := statusChangeView{Label: label, Class: class}
		if changedAt, err := time.Parse(time.RFC3339Nano, change.ChangedAt); err == nil {
			view.Date = changedAt.In(applicationLocation).Format("Jan 2, 2006")
			view.Ago = relativeDays(calendarDaysBetween(changedAt.In(now.Location()), now))
		}
		views = append(views, view)
	}
	encoded, err := json.Marshal(views)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

func formatTimestampDate(value string) string {
	parsedTime, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return ""
	}
	return parsedTime.In(applicationLocation).Format("Jan 2, 2006")
}

func formatStatus(status string) (string, string) {
	switch status {
	case "in_contact":
		return "Interviewing", "status-contact"
	case "offer":
		return "Offer", "status-offer"
	case "rejected_after_contact":
		return "Rejected", "status-rejected"
	case "rejected_no_contact":
		return "No response", "status-closed"
	case "withdrawn":
		return "Withdrawn", "status-withdrawn"
	default:
		return "Applied", "status-applied"
	}
}

func formatLastChecked(lastChecked sql.NullString, now time.Time, portalCheckDays int) (string, string, bool) {
	if !lastChecked.Valid {
		return "Never checked", "", true
	}

	checkedAt, err := time.Parse(time.RFC3339Nano, lastChecked.String)
	if err != nil {
		return lastChecked.String, "", false
	}
	checkedAt = checkedAt.In(now.Location())

	daysAgo := calendarDaysBetween(checkedAt, now)
	return relativeDays(daysAgo), checkedAt.Format("Jan 2, 3:04 PM"), daysAgo >= 1 && daysAgo >= portalCheckDays
}

func relativeDays(days int) string {
	switch {
	case days < 1:
		return "Today"
	case days == 1:
		return "Yesterday"
	default:
		return fmt.Sprintf("%d days ago", days)
	}
}

func calendarDaysBetween(earlier, later time.Time) int {
	earlierYear, earlierMonth, earlierDay := earlier.Date()
	laterYear, laterMonth, laterDay := later.Date()
	earlierDate := time.Date(earlierYear, earlierMonth, earlierDay, 0, 0, 0, 0, time.UTC)
	laterDate := time.Date(laterYear, laterMonth, laterDay, 0, 0, 0, 0, time.UTC)
	return int(laterDate.Sub(earlierDate) / (24 * time.Hour))
}
