package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"io"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/pokemastercp/jobby/internal/database"
	"github.com/pressly/goose/v3"
)

func TestNeedsPortalCheck(t *testing.T) {
	cases := []struct {
		status string
		portal string
		want   bool
	}{
		{status: "applied", portal: "https://jobs.example", want: true},
		{status: "in_contact", portal: "  https://jobs.example/openings  ", want: true},
		{status: "applied", portal: "", want: false},
		{status: "applied", portal: "   \t\n", want: false},
		{status: "in_contact", portal: "", want: false},
		{status: "in_contact", portal: " ", want: false},
		{status: "offer", portal: "https://jobs.example", want: false},
		{status: "rejected_after_contact", portal: "https://jobs.example", want: false},
		{status: "rejected_no_contact", portal: "https://jobs.example", want: false},
		{status: "withdrawn", portal: "https://jobs.example", want: false},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s/%q", tc.status, tc.portal), func(t *testing.T) {
			if got := needsPortalCheck(tc.status, tc.portal); got != tc.want {
				t.Fatalf("needsPortalCheck(%q, %q) = %v, want %v", tc.status, tc.portal, got, tc.want)
			}
		})
	}
}

func TestCareerPortalURL(t *testing.T) {
	cases := []struct {
		name  string
		value sql.NullString
		want  string
	}{
		{name: "null", value: sql.NullString{}, want: ""},
		{name: "empty", value: sql.NullString{String: "", Valid: true}, want: ""},
		{name: "whitespace", value: sql.NullString{String: " \t\n ", Valid: true}, want: ""},
		{name: "padded", value: sql.NullString{String: "  https://jobs.example/x  ", Valid: true}, want: "https://jobs.example/x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := careerPortalURL(tc.value); got != tc.want {
				t.Fatalf("careerPortalURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPortalCheckSelectsOnlyOpenApplicationsWithPortal(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	seedPortalCheckFixtures(t, db)
	queries := database.New(db)

	overview, err := loadOverviewData(ctx, queries)
	if err != nil {
		t.Fatal(err)
	}

	var selected []string
	for _, due := range overview.DueApplications {
		selected = append(selected, due.Application.OrganizationName+" / "+due.Application.RoleTitle)
	}
	wantSelected := []string{
		"Padded Portal / Research Scientist",
		"Northwind / Staff Engineer",
		"Northwind / Recruiter Screen",
		"Northwind / Seven Day Check",
	}
	if strings.Join(selected, "\n") != strings.Join(wantSelected, "\n") {
		t.Fatalf("portal check selected:\n%s\nwant:\n%s", strings.Join(selected, "\n"), strings.Join(wantSelected, "\n"))
	}
	if overview.DueCount != len(wantSelected) {
		t.Fatalf("DueCount = %d, want %d", overview.DueCount, len(wantSelected))
	}
	if overview.QuietDueCount != 2 {
		t.Fatalf("QuietDueCount = %d, want 2", overview.QuietDueCount)
	}
	if overview.CheckedTodayCount != 1 || overview.RoundTotal != 5 {
		t.Fatalf("round progress = %d of %d, want 1 of 5", overview.CheckedTodayCount, overview.RoundTotal)
	}
	if overview.NextCheck != "Next up: Northwind tomorrow." {
		t.Fatalf("NextCheck = %q", overview.NextCheck)
	}
	if overview.OpenCount != 10 {
		t.Fatalf("OpenCount = %d, want 10", overview.OpenCount)
	}

	templates, err := template.ParseFS(appFS, "web/templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	var overviewHTML bytes.Buffer
	if err := templates.ExecuteTemplate(&overviewHTML, "index.html", overview); err != nil {
		t.Fatal(err)
	}
	round := between(overviewHTML.String(), `id="round-title"`, `id="conversations-title"`)
	if !strings.Contains(round, "4 portals to check") && !strings.Contains(overviewHTML.String(), "4 portals to check") {
		t.Fatal("today page did not show 4 portals to check")
	}
	for _, role := range []string{"Research Scientist", "Staff Engineer", "Recruiter Screen", "Seven Day Check"} {
		if !strings.Contains(round, role) {
			t.Fatalf("portal round missing %s", role)
		}
	}
	for _, role := range []string{"Copywriter", "Data Analyst", "Product Designer", "Account Executive", "Director", "Walked Away", "Six Day Check", "Checked Today"} {
		if strings.Contains(round, role) {
			t.Fatalf("portal round included %s", role)
		}
	}
	if !strings.Contains(round, `href="https://jobs.padded.test/openings"`) {
		t.Fatal("padded portal URL was not trimmed in the round")
	}
	conversations := between(overviewHTML.String(), `id="conversations-title"`, `id="activity-title"`)
	if strings.Contains(between(conversations, "Product Designer", "</li>"), "Check due") {
		t.Fatal("interview without a portal was marked check due")
	}
	if !strings.Contains(between(conversations, "Recruiter Screen", "</li>"), "Check due") {
		t.Fatal("interview with a due portal was not marked check due")
	}

	applications, err := loadApplicationsPageData(ctx, queries, applicationFilters{})
	if err != nil {
		t.Fatal(err)
	}
	if applications.DueCount != len(wantSelected) {
		t.Fatalf("applications DueCount = %d, want %d", applications.DueCount, len(wantSelected))
	}
	var applicationsHTML bytes.Buffer
	if err := templates.ExecuteTemplate(&applicationsHTML, "applications.html", applications); err != nil {
		t.Fatal(err)
	}
	page := applicationsHTML.String()
	if !strings.Contains(page, "4 portal checks due today") {
		t.Fatal("applications page did not show 4 portal checks due today")
	}
	assertApplicationCheck := func(role, portalCheck, due, careersURL string) {
		t.Helper()
		attrs := applicationAttributes(page, role)
		if attrs == "" {
			t.Fatalf("applications page missing %s", role)
		}
		for _, want := range []string{
			`data-portal-check="` + portalCheck + `"`,
			`data-due="` + due + `"`,
			`data-careers-url="` + careersURL + `"`,
		} {
			if !strings.Contains(attrs, want) {
				t.Fatalf("%s attributes missing %s\n%s", role, want, attrs)
			}
		}
	}
	assertApplicationCheck("Research Scientist", "true", "true", "https://jobs.padded.test/openings")
	assertApplicationCheck("Staff Engineer", "true", "true", "https://careers.northwind.test")
	assertApplicationCheck("Recruiter Screen", "true", "true", "https://careers.northwind.test")
	assertApplicationCheck("Seven Day Check", "true", "true", "https://careers.northwind.test")
	assertApplicationCheck("Six Day Check", "true", "false", "https://careers.northwind.test")
	assertApplicationCheck("Checked Today", "true", "false", "https://careers.northwind.test")
	assertApplicationCheck("Copywriter", "false", "false", "")
	assertApplicationCheck("Data Analyst", "false", "false", "")
	assertApplicationCheck("Product Designer", "false", "false", "")
	assertApplicationCheck("Director", "false", "false", "https://careers.northwind.test")
	assertApplicationCheck("Account Executive", "false", "false", "https://careers.closed.test")
	assertApplicationCheck("Walked Away", "false", "false", "")
	if strings.Count(page, `title="No career portal saved for this organization"`) != 3 {
		t.Fatalf("no-portal marker count = %d, want 3", strings.Count(page, `title="No career portal saved for this organization"`))
	}

	organizations, err := loadOrganizationsPageData(ctx, queries, organizationsPageState{})
	if err != nil {
		t.Fatal(err)
	}
	if organizations.WithCareerPortal != 3 || organizations.MissingPortalCount != 3 {
		t.Fatalf("organizations with portal = %d, missing = %d; want 3 and 3", organizations.WithCareerPortal, organizations.MissingPortalCount)
	}
	for _, organization := range organizations.Organizations {
		switch organization.Name {
		case "Whitespace Portal", "Blank Portal", "No Portal Co":
			if organization.CareersURL != "" || !organization.NeedsPortal {
				t.Fatalf("%s careers URL = %q, needs portal = %v", organization.Name, organization.CareersURL, organization.NeedsPortal)
			}
		case "Padded Portal":
			if organization.CareersURL != "https://jobs.padded.test/openings" || organization.NeedsPortal {
				t.Fatalf("padded portal stored as %q, needs portal = %v", organization.CareersURL, organization.NeedsPortal)
			}
		}
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", fmt.Sprintf("file:jobby-test-%d?mode=memory&cache=shared", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}
	goose.SetBaseFS(appFS)
	goose.SetLogger(log.New(io.Discard, "", 0))
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	if err := goose.Up(db, "db/migrations"); err != nil {
		t.Fatal(err)
	}
	return db
}

func seedPortalCheckFixtures(t *testing.T, db *sql.DB) {
	t.Helper()
	orgs := []struct {
		name   string
		portal any
	}{
		{name: "Northwind", portal: "https://careers.northwind.test"},
		{name: "No Portal Co", portal: nil},
		{name: "Blank Portal", portal: ""},
		{name: "Whitespace Portal", portal: " \t\n "},
		{name: "Padded Portal", portal: "  https://jobs.padded.test/openings  "},
		{name: "Closed Shop", portal: "https://careers.closed.test"},
	}
	ids := make(map[string]int64, len(orgs))
	for _, org := range orgs {
		result, err := db.Exec(`INSERT INTO organizations (name, careers_url) VALUES (?, ?)`, org.name, org.portal)
		if err != nil {
			t.Fatal(err)
		}
		id, err := result.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		ids[org.name] = id
	}

	type app struct {
		org            string
		role           string
		status         string
		appliedDaysAgo int
		checkedDaysAgo *int
		createdDaysAgo int
	}
	days := func(n int) *int { return &n }
	apps := []app{
		{org: "Northwind", role: "Staff Engineer", status: "applied", appliedDaysAgo: 40, checkedDaysAgo: days(12), createdDaysAgo: 40},
		{org: "Northwind", role: "Recruiter Screen", status: "in_contact", appliedDaysAgo: 20, checkedDaysAgo: days(10), createdDaysAgo: 20},
		{org: "Northwind", role: "Seven Day Check", status: "applied", appliedDaysAgo: 5, checkedDaysAgo: days(7), createdDaysAgo: 5},
		{org: "Northwind", role: "Six Day Check", status: "applied", appliedDaysAgo: 6, checkedDaysAgo: days(6), createdDaysAgo: 6},
		{org: "Northwind", role: "Checked Today", status: "applied", appliedDaysAgo: 4, checkedDaysAgo: days(0), createdDaysAgo: 4},
		{org: "Northwind", role: "Director", status: "offer", appliedDaysAgo: 50, checkedDaysAgo: nil, createdDaysAgo: 50},
		{org: "Padded Portal", role: "Research Scientist", status: "applied", appliedDaysAgo: 45, checkedDaysAgo: nil, createdDaysAgo: 45},
		{org: "No Portal Co", role: "Copywriter", status: "applied", appliedDaysAgo: 30, checkedDaysAgo: days(20), createdDaysAgo: 30},
		{org: "Blank Portal", role: "Data Analyst", status: "applied", appliedDaysAgo: 15, checkedDaysAgo: nil, createdDaysAgo: 15},
		{org: "Whitespace Portal", role: "Product Designer", status: "in_contact", appliedDaysAgo: 25, checkedDaysAgo: days(14), createdDaysAgo: 25},
		{org: "Closed Shop", role: "Account Executive", status: "rejected_no_contact", appliedDaysAgo: 60, checkedDaysAgo: days(20), createdDaysAgo: 60},
		{org: "Blank Portal", role: "Walked Away", status: "withdrawn", appliedDaysAgo: 12, checkedDaysAgo: days(9), createdDaysAgo: 12},
	}
	for _, application := range apps {
		var checked any
		if application.checkedDaysAgo != nil {
			checked = stamp(*application.checkedDaysAgo)
		}
		created := stamp(application.createdDaysAgo)
		_, err := db.Exec(`
			INSERT INTO applications (
				organization_id, role_title, status, work_location, applied_at,
				status_changed_at, last_checked_at, created_at, updated_at
			) VALUES (?, ?, ?, 'remote', ?, ?, ?, ?, ?)
		`, ids[application.org], application.role, application.status, dayStamp(application.appliedDaysAgo), created, checked, created, created)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func stamp(days int) string {
	return portalClock(days).Format("2006-01-02T15:04:05.000Z")
}

func dayStamp(days int) string {
	return portalClock(days).Format("2006-01-02")
}

func portalClock(daysAgo int) time.Time {
	now := time.Now().In(applicationLocation)
	year, month, day := now.Date()
	return time.Date(year, month, day, 16, 0, 0, 0, time.UTC).AddDate(0, 0, -daysAgo)
}

func between(value, start, end string) string {
	i := strings.Index(value, start)
	if i < 0 {
		return ""
	}
	value = value[i:]
	j := strings.Index(value, end)
	if j < 0 {
		return value
	}
	return value[:j]
}

func applicationAttributes(page, role string) string {
	needle := `data-role-title="` + role + `"`
	i := strings.Index(page, needle)
	if i < 0 {
		return ""
	}
	start := strings.LastIndex(page[:i], "<")
	end := strings.Index(page[i:], ">")
	if start < 0 || end < 0 {
		return ""
	}
	return page[start : i+end]
}
