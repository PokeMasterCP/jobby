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

	"github.com/pokemastercp/jobby/internal/database"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed db/migrations/*.sql web/templates/*.html web/static
var appFS embed.FS

var applicationLocation = mustLoadLocation("America/Chicago")

type overviewData struct {
	Applications              []applicationView
	PortalApplications        []portalApplicationView
	OrganizationNames         []string
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
	ApplicationForm           applicationFormView
	EditApplicationForm       applicationFormView
	SelectedApplicationID     int64
}

type applicationsPageData struct {
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
	ID               int64
	OrganizationName string
	LastChecked      string
	daysSinceCheck   int
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

type organizationFormView struct {
	ID              int64
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
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		renderOverview(w, r, queries, templates, overviewPageState{}, http.StatusOK)
	})
	mux.HandleFunc("GET /applications", func(w http.ResponseWriter, r *http.Request) {
		renderApplications(w, r.Context(), queries, templates, parseApplicationFilters(r.URL.Query()), applicationsPageState{}, http.StatusOK)
	})
	mux.HandleFunc("GET /organizations/{id}/edit", func(w http.ResponseWriter, r *http.Request) {
		organizationID, err := parseOrganizationID(r)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		organization, err := queries.GetOrganization(r.Context(), organizationID)
		if err == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			log.Printf("get organization: %v", err)
			http.Error(w, "Unable to load organization", http.StatusInternalServerError)
			return
		}

		form := newOrganizationFormView(organization)
		form.Saved = r.URL.Query().Get("saved") == "1"
		renderOrganizationForm(w, templates, form, http.StatusOK)
	})
	mux.HandleFunc("POST /organizations/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !isSameOrigin(r) {
			http.Error(w, "Invalid request origin", http.StatusForbidden)
			return
		}

		organizationID, err := parseOrganizationID(r)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		form, input := parseOrganizationForm(r)
		form.ID = organizationID
		if form.HasErrors {
			renderOrganizationForm(w, templates, form, http.StatusUnprocessableEntity)
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
			renderOrganizationForm(w, templates, form, http.StatusUnprocessableEntity)
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

		http.Redirect(w, r, fmt.Sprintf("/organizations/%d/edit?saved=1", organizationID), http.StatusSeeOther)
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

		applicationID, err := parseApplicationID(r)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		form, input := parseApplicationForm(r, true)
		if form.HasErrors {
			if referer, ok := applicationsPageReferer(r); ok {
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

		applicationID, err := parseApplicationID(r)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
		status := strings.TrimSpace(r.FormValue("status"))
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

		applicationID, err := parseApplicationID(r)
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

		applicationID, err := parseApplicationID(r)
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

func renderOrganizationForm(w http.ResponseWriter, templates *template.Template, form organizationFormView, status int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := templates.ExecuteTemplate(w, "organization-edit.html", form); err != nil {
		log.Printf("render organization form: %v", err)
	}
}

func loadOverviewData(ctx context.Context, queries *database.Queries) (overviewData, error) {
	applications, err := queries.ListApplications(ctx)
	if err != nil {
		return overviewData{}, err
	}

	data := overviewData{
		TotalApplications: len(applications),
	}
	organizations := make(map[int64]string)
	now := time.Now().In(applicationLocation)
	for _, application := range applications {
		organizations[application.OrganizationID] = application.OrganizationName

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
	for _, organizationName := range organizations {
		data.OrganizationNames = append(data.OrganizationNames, organizationName)
	}
	sort.Strings(data.OrganizationNames)

	data.ApplicationSummary = fmt.Sprintf("Showing all %d open applications", data.OpenCount)
	if data.OpenCount == 1 {
		data.ApplicationSummary = "Showing 1 open application"
	} else if len(data.Applications) > 10 {
		data.ApplicationSummary = fmt.Sprintf("Showing 10 most recent of %d open applications", data.OpenCount)
		data.Applications = data.Applications[:10]
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

	return data, nil
}

func loadApplicationsPageData(ctx context.Context, queries *database.Queries, filters applicationFilters) (applicationsPageData, error) {
	applications, err := queries.ListApplications(ctx)
	if err != nil {
		return applicationsPageData{}, err
	}

	data := applicationsPageData{
		TotalApplications: len(applications),
		Filters:           filters,
	}
	organizations := make(map[int64]string)
	now := time.Now().In(applicationLocation)
	for _, application := range applications {
		organizations[application.OrganizationID] = application.OrganizationName
		if applicationMatchesFilters(application, filters) {
			data.Applications = append(data.Applications, newApplicationView(application, now))
		}
	}

	data.OrganizationCount = len(organizations)
	for id, name := range organizations {
		data.OrganizationOptions = append(data.OrganizationOptions, organizationFilterOption{ID: id, Name: name})
	}
	sort.Slice(data.OrganizationOptions, func(i, j int) bool {
		return data.OrganizationOptions[i].Name < data.OrganizationOptions[j].Name
	})

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

func applicationsPageReferer(r *http.Request) (*url.URL, bool) {
	referer, err := url.Parse(r.Referer())
	if err != nil || referer.Path != "/applications" {
		return nil, false
	}
	return referer, true
}

func applicationMutationReturnPath(r *http.Request) string {
	if referer, ok := applicationsPageReferer(r); ok {
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

func newOrganizationFormView(organization database.Organization) organizationFormView {
	return organizationFormView{
		ID:         organization.ID,
		Name:       organization.Name,
		CareersURL: nullStringValue(organization.CareersUrl),
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

func parseApplicationID(r *http.Request) (int64, error) {
	applicationID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || applicationID < 1 {
		return 0, fmt.Errorf("invalid application id")
	}
	return applicationID, nil
}

func parseOrganizationID(r *http.Request) (int64, error) {
	organizationID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || organizationID < 1 {
		return 0, fmt.Errorf("invalid organization id")
	}
	return organizationID, nil
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

func newApplicationView(application database.ListApplicationsRow, now time.Time) applicationView {
	lastChecked, lastCheckedDetail, lastCheckedIsDue := formatLastChecked(application.LastCheckedAt, now)
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

func newPortalApplicationView(application database.ListApplicationsRow, now time.Time) portalApplicationView {
	if !application.LastCheckedAt.Valid {
		return portalApplicationView{
			ID:               application.ID,
			OrganizationName: application.OrganizationName,
			LastChecked:      "Never",
			daysSinceCheck:   int(^uint(0) >> 1),
		}
	}

	checkedAt, _ := time.Parse(time.RFC3339Nano, application.LastCheckedAt.String)
	daysSinceCheck := calendarDaysBetween(checkedAt.In(now.Location()), now)
	return portalApplicationView{
		ID:               application.ID,
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

func formatLastChecked(lastChecked sql.NullString, now time.Time) (string, string, bool) {
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
	if daysAgo == 1 {
		return "Yesterday", checkedAt.Format("3:04 PM"), false
	}
	return fmt.Sprintf("%d days ago", daysAgo), "Check portal ↗", daysAgo >= 7
}

func calendarDaysBetween(earlier, later time.Time) int {
	earlierYear, earlierMonth, earlierDay := earlier.Date()
	laterYear, laterMonth, laterDay := later.Date()
	earlierDate := time.Date(earlierYear, earlierMonth, earlierDay, 0, 0, 0, 0, time.UTC)
	laterDate := time.Date(laterYear, laterMonth, laterDay, 0, 0, 0, 0, time.UTC)
	return int(laterDate.Sub(earlierDate) / (24 * time.Hour))
}
