package calendar

import "testing"

func TestProblem(t *testing.T) {
	ok := AccountStatus{Name: "ok", Connected: true}
	for _, tc := range []struct {
		name     string
		accounts []AccountStatus
		want     string
	}{
		{"no accounts", nil, ""},
		{"all fine", []AccountStatus{ok}, ""},
		{"not read yet", []AccountStatus{{Name: "new"}}, ""},
		{"network", []AccountStatus{ok, {Name: "w", Connected: true, Error: "timeout"}}, ProblemUnavailable},
		{"never signed in", []AccountStatus{{Name: "w", Error: "not connected"}}, ProblemNotConnected},
		{"expired wins", []AccountStatus{
			{Name: "a", Error: "not connected"},
			{Name: "b", Connected: true, LoginExpired: true, Error: "sign-in expired or revoked"},
			{Name: "c", Connected: true, Error: "timeout"},
		}, ProblemLoginExpired},
	} {
		if got := Problem(tc.accounts); got != tc.want {
			t.Errorf("%s: Problem = %q, want %q", tc.name, got, tc.want)
		}
	}
}
