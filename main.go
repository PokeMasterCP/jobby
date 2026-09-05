package main

import (
	"context"
	"database/sql"
	"embed"
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

type overviewData struct {
	Settings                  database.GetSettingsRow
	ReturnPath                string
	Applications              []applicationView
	PortalApplications        []portalApplicationView
	OrganizationNames         []string
	DashboardWeeks            []dashboardWeekView
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
	ApplicationsLast7         int
	ApplicationsLast30        int
	ReachedContactCount       int
	ContactRate               int
	ClosedCount               int
	OfferRate                 int
	NoResponseRate            int
	MedianOpenAge             string
	SalaryListedCount         int
	SalaryCoverage            int
	MedianOpenSalary          string
	RemoteCount               int
	LocalCount                int
	RemotePercent             int
	LocalPercent              int
	ApplicationForm           applicationFormView
	EditApplicationForm       applicationFormView
	SelectedApplicationID     int64
}

type dashboardWeekView struct {
	Label     string
	Count     int
	Height    int
	IsCurrent bool
}

type applicationsPageData struct {
	Settings              database.GetSettingsRow
	ReturnPath            string
	Applications          []applicationView
	OrganizationOptions   []organizationFilterOption
	TotalApplications     int
	OrganizationCount     int
	FilteredCount         int
	FilterSummary         string
	Filters               applicationFilters
	EditApplicationForm   applicationFormView
	SelectedApplicationID int64
}

type organizationsPageData struct {
	Settings               database.GetSettingsRow
	Organizations          []organizationView
	TotalOrganizations     int
	TotalApplications      int64
	OpenApplications       int64
	WithCareerPortal       int
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
	UpdatedAt            string
}

type applicationFilters struct {
	Status         string
	Income         string
	OrganizationID int64
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
	LocationIcon        string
	WorkLocation        string
	StatusLabel         string
	StatusClass         string
	CareersURL          string
	PostingURL          string
	AppliedAt           string
	AppliedAtDisplay    string
	LastChecked         string
	LastCheckedDetail   string
	LastCheckedIsDue    bool
	Notes               string
}

type portalApplicationView struct {
	Application    applicationView
	CheckAge       string
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
		if !utf8.ValidString(name) || utf8.RuneCountInString(name) > 100 {
			http.Error(w, "Name must be 100 characters or fewer.", http.StatusUnprocessableEntity)
			return
		}
		if err != nil || days < 1 || days > 365 {
			http.Error(w, "Choose a portal check interval from 1 to 365 whole days.", http.StatusUnprocessableEntity)
			return
		}
		if err := queries.UpdateSettings(r.Context(), database.UpdateSettingsParams{Name: name, PortalCheckDays: days}); err != nil {
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
			renderOverview(w, r, queries, templates, overviewPageState{ApplicationForm: form}, http.StatusUnprocessableEntity)
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

		_, err = txQueries.CreateApplication(r.Context(), database.CreateApplicationParams{
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

		if err := tx.Commit(); err != nil {
			log.Printf("commit create application: %v", err)
			http.Error(w, "Unable to save application", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/", http.StatusSeeOther)
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

		rowsAffected, err := queries.UpdateApplicationStatus(r.Context(), database.UpdateApplicationStatusParams{
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

	addr := ":8080"
	log.Printf("listening on http://localhost%s", addr)
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

	data := overviewData{
		Settings:          settings,
		ReturnPath:        "/#applications",
		TotalApplications: len(applications),
		OrganizationCount: len(organizations),
		DashboardWeeks:    make([]dashboardWeekView, 8),
		MedianOpenAge:     "—",
		MedianOpenSalary:  "—",
	}
	for _, organization := range organizations {
		data.OrganizationNames = append(data.OrganizationNames, organization.Name)
	}

	now := time.Now().In(applicationLocation)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, applicationLocation)
	weekStart := today.AddDate(0, 0, -(int(today.Weekday())+6)%7)
	firstWeekStart := weekStart.AddDate(0, 0, -49)
	weekCounts := make([]int, len(data.DashboardWeeks))
	openAges := make([]int, 0, len(applications))
	openSalaryMidpoints := make([]int64, 0, len(applications))

	for index := range data.DashboardWeeks {
		label := firstWeekStart.AddDate(0, 0, index*7).Format("Jan 2")
		if index == len(data.DashboardWeeks)-1 {
			label = "This week"
		}
		data.DashboardWeeks[index] = dashboardWeekView{
			Label:     label,
			IsCurrent: index == len(data.DashboardWeeks)-1,
		}
	}

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
			}
			if !activityDay.Before(firstWeekStart) && !activityDay.After(today) {
				weekIndex := calendarDaysBetween(firstWeekStart, activityDay) / 7
				if weekIndex >= 0 && weekIndex < len(weekCounts) {
					weekCounts[weekIndex]++
				}
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
		if hasActivityDate && !activityDate.After(now) {
			openAges = append(openAges, calendarDaysBetween(activityDate, now))
		}
		if application.SalaryMin.Valid && application.SalaryMax.Valid {
			openSalaryMidpoints = append(openSalaryMidpoints, application.SalaryMin.Int64+(application.SalaryMax.Int64-application.SalaryMin.Int64)/2)
		}
		applicationView := newApplicationView(application, now, int(settings.PortalCheckDays))
		data.Applications = append(data.Applications, applicationView)
		if applicationView.LastCheckedIsDue {
			data.PortalApplications = append(data.PortalApplications, newPortalApplicationView(application, applicationView, now))
		}
	}

	data.ReachedContactCount = data.InContactCount + data.AcceptedCount + data.RejectedAfterContactCount
	data.ContactRate = roundedPercentage(data.ReachedContactCount, data.TotalApplications)
	data.ClosedCount = data.AcceptedCount + data.RejectedAfterContactCount + data.RejectedNoContactCount
	data.OfferRate = roundedPercentage(data.AcceptedCount, data.ClosedCount)
	data.NoResponseRate = roundedPercentage(data.RejectedNoContactCount, data.ClosedCount)
	data.SalaryCoverage = roundedPercentage(data.SalaryListedCount, data.TotalApplications)
	data.RemotePercent = roundedPercentage(data.RemoteCount, data.TotalApplications)
	data.LocalPercent = roundedPercentage(data.LocalCount, data.TotalApplications)

	if len(openAges) > 0 {
		sort.Ints(openAges)
		data.MedianOpenAge = fmt.Sprintf("%dd", medianInt(openAges))
	}
	if len(openSalaryMidpoints) > 0 {
		sort.Slice(openSalaryMidpoints, func(i, j int) bool {
			return openSalaryMidpoints[i] < openSalaryMidpoints[j]
		})
		data.MedianOpenSalary = compactSalary(medianInt64(openSalaryMidpoints))
	}

	maxWeekCount := 0
	for _, count := range weekCounts {
		if count > maxWeekCount {
			maxWeekCount = count
		}
	}
	for index, count := range weekCounts {
		height := 0
		if maxWeekCount > 0 && count > 0 {
			height = max(12, count*100/maxWeekCount)
		}
		data.DashboardWeeks[index].Count = count
		data.DashboardWeeks[index].Height = height
	}

	data.ApplicationSummary = fmt.Sprintf("Showing all %d open applications", data.OpenCount)
	if data.OpenCount == 1 {
		data.ApplicationSummary = "Showing 1 open application"
	} else if len(data.Applications) > 5 {
		data.ApplicationSummary = fmt.Sprintf("Showing 5 most recent of %d open applications", data.OpenCount)
		data.Applications = data.Applications[:5]
	}

	sort.Slice(data.PortalApplications, func(i, j int) bool {
		return data.PortalApplications[i].daysSinceCheck > data.PortalApplications[j].daysSinceCheck
	})
	data.PortalHeadingCount, data.PortalHeadingAction, data.PortalCheckSummary = portalCopy(len(data.PortalApplications))
	data.PortalDescription = fmt.Sprintf("Check open applications every %d days. These applications are due for a check.", settings.PortalCheckDays)
	if settings.PortalCheckDays == 1 {
		data.PortalDescription = "Check open applications daily. These applications are due for a check."
	}
	if len(data.PortalApplications) == 0 {
		data.PortalDescription = "You're caught up on portal checks for every open application."
	}
	if len(data.PortalApplications) > 3 {
		data.PortalApplications = data.PortalApplications[:3]
	}

	return data, nil
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

	returnQuery := url.Values{}
	if filters.Status != "" {
		returnQuery.Set("status", filters.Status)
	}
	if filters.Income != "" {
		returnQuery.Set("income", filters.Income)
	}
	if filters.OrganizationID != 0 {
		returnQuery.Set("organization", strconv.FormatInt(filters.OrganizationID, 10))
	}
	returnURL := url.URL{Path: "/applications", RawQuery: returnQuery.Encode()}
	data := applicationsPageData{
		Settings:          settings,
		ReturnPath:        returnURL.RequestURI(),
		TotalApplications: len(applications),
		OrganizationCount: len(organizations),
		Filters:           filters,
	}
	for _, organization := range organizations {
		data.OrganizationOptions = append(data.OrganizationOptions, organizationFilterOption{
			ID:   organization.ID,
			Name: organization.Name,
		})
	}
	now := time.Now().In(applicationLocation)
	for _, application := range applications {
		if applicationMatchesFilters(application, filters) {
			data.Applications = append(data.Applications, newApplicationView(application, now, int(settings.PortalCheckDays)))
		}
	}

	data.FilteredCount = len(data.Applications)
	data.FilterSummary = fmt.Sprintf("Showing all %d applications", data.TotalApplications)
	if data.TotalApplications == 1 {
		data.FilterSummary = "Showing 1 application"
	}
	if filters.Status != "" || filters.Income != "" || filters.OrganizationID != 0 {
		data.FilterSummary = fmt.Sprintf("Showing %d of %d applications", data.FilteredCount, data.TotalApplications)
	}

	return data, nil
}

func loadOrganizationsPageData(ctx context.Context, queries *database.Queries, state organizationsPageState) (organizationsPageData, error) {
	settings, err := queries.GetSettings(ctx)
	if err != nil {
		return organizationsPageData{}, err
	}
	organizations, err := queries.ListOrganizations(ctx)
	if err != nil {
		return organizationsPageData{}, err
	}

	data := organizationsPageData{
		Settings:               settings,
		TotalOrganizations:     len(organizations),
		EditOrganizationForm:   state.EditOrganizationForm,
		SelectedOrganizationID: state.SelectedOrganizationID,
	}
	foundSelection := false
	for _, organization := range organizations {
		view := newOrganizationView(organization)
		data.Organizations = append(data.Organizations, view)
		data.TotalApplications += organization.ApplicationCount
		data.OpenApplications += organization.OpenApplicationCount
		if view.CareersURL != "" {
			data.WithCareerPortal++
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
	if status == "open" || isApplicationStatus(status) {
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
	return filters
}

func applicationMatchesFilters(application database.ListApplicationsRow, filters applicationFilters) bool {
	if filters.Status == "open" && !isOpenStatus(application.Status) {
		return false
	}
	if filters.Status != "" && filters.Status != "open" && application.Status != filters.Status {
		return false
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
	return "/#applications"
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

func newOrganizationView(organization database.ListOrganizationsRow) organizationView {
	careersURL := nullStringValue(organization.CareersUrl)
	portalLabel := "Not saved"
	if careersURL != "" {
		portalLabel = "Portal saved"
		if parsedURL, err := url.Parse(careersURL); err == nil && parsedURL.Hostname() != "" {
			portalLabel = strings.TrimPrefix(parsedURL.Hostname(), "www.")
		}
	}

	updatedAt := "Recently updated"
	if parsedTime, err := time.Parse(time.RFC3339Nano, organization.UpdatedAt); err == nil {
		updatedAt = "Updated " + parsedTime.In(applicationLocation).Format("Jan 2, 2006")
	}

	return organizationView{
		ID:                   organization.ID,
		Name:                 organization.Name,
		Initial:              organizationInitial(organization.Name),
		Mark:                 organizationMark(organization.ID),
		CareersURL:           careersURL,
		PortalLabel:          portalLabel,
		ApplicationCount:     organization.ApplicationCount,
		OpenApplicationCount: organization.OpenApplicationCount,
		UpdatedAt:            updatedAt,
	}
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
	return status == "applied" || status == "in_contact"
}

func isApplicationStatus(status string) bool {
	switch status {
	case "applied", "in_contact", "accepted", "rejected_after_contact", "rejected_no_contact":
		return true
	default:
		return false
	}
}

func newApplicationView(application database.ListApplicationsRow, now time.Time, portalCheckDays int) applicationView {
	lastChecked, lastCheckedDetail, lastCheckedIsDue := formatLastChecked(application.LastCheckedAt, now, portalCheckDays)
	statusLabel, statusClass := formatStatus(application.Status)

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
		LocationIcon:        map[string]string{"remote": "⌂", "local": "⌖"}[application.WorkLocation],
		WorkLocation:        application.WorkLocation,
		StatusLabel:         statusLabel,
		StatusClass:         statusClass,
		CareersURL:          nullStringValue(application.OrganizationCareersUrl),
		PostingURL:          nullStringValue(application.PostingUrl),
		AppliedAt:           nullStringValue(application.AppliedAt),
		AppliedAtDisplay:    formatAppliedAt(application.AppliedAt),
		LastChecked:         lastChecked,
		LastCheckedDetail:   lastCheckedDetail,
		LastCheckedIsDue:    lastCheckedIsDue,
		Notes:               nullStringValue(application.Notes),
	}
}

func newPortalApplicationView(application database.ListApplicationsRow, view applicationView, now time.Time) portalApplicationView {
	if !application.LastCheckedAt.Valid {
		return portalApplicationView{
			Application:    view,
			CheckAge:       "Never",
			daysSinceCheck: int(^uint(0) >> 1),
		}
	}

	checkedAt, _ := time.Parse(time.RFC3339Nano, application.LastCheckedAt.String)
	daysSinceCheck := calendarDaysBetween(checkedAt.In(now.Location()), now)
	return portalApplicationView{
		Application:    view,
		CheckAge:       fmt.Sprintf("%dd", daysSinceCheck),
		daysSinceCheck: daysSinceCheck,
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

func formatLastChecked(lastChecked sql.NullString, now time.Time, portalCheckDays int) (string, string, bool) {
	if !lastChecked.Valid {
		return "Not checked", "Check portal ↗", true
	}

	checkedAt, err := time.Parse(time.RFC3339Nano, lastChecked.String)
	if err != nil {
		return lastChecked.String, "", false
	}
	checkedAt = checkedAt.In(now.Location())

	daysAgo := calendarDaysBetween(checkedAt, now)
	if daysAgo < 1 {
		return "Today", checkedAt.Format("3:04 PM"), false
	}
	isDue := daysAgo >= portalCheckDays
	detail := checkedAt.Format("3:04 PM")
	if isDue {
		detail = "Check portal ↗"
	}
	if daysAgo == 1 {
		return "Yesterday", detail, isDue
	}
	return fmt.Sprintf("%d days ago", daysAgo), detail, isDue
}

func calendarDaysBetween(earlier, later time.Time) int {
	earlierYear, earlierMonth, earlierDay := earlier.Date()
	laterYear, laterMonth, laterDay := later.Date()
	earlierDate := time.Date(earlierYear, earlierMonth, earlierDay, 0, 0, 0, 0, time.UTC)
	laterDate := time.Date(laterYear, laterMonth, laterDay, 0, 0, 0, 0, time.UTC)
	return int(laterDate.Sub(earlierDate) / (24 * time.Hour))
}
