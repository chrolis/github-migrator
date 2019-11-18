package migrator

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/itchyny/github-migrator/github"
	"github.com/itchyny/github-migrator/repo"
)

func init() {
	beforeImportIssueDuration = 0
	waitImportIssueInitialDuration = 0
}

type testRepo struct {
	Repo         *github.Repo
	UpdateRepo   *github.Repo     `json:"update_repo"`
	Members      []*github.Member `json:"members"`
	Labels       []*github.Label  `json:"labels"`
	CreateLabels []*github.Label  `json:"create_labels"`
	UpdateLabels []*github.Label  `json:"update_labels"`
	Issues       []struct {
		*github.PullReq
		Comments       []*github.Comment       `json:"comments"`
		Events         []*github.Event         `json:"events"`
		Commits        []*github.Commit        `json:"commit_details"`
		Reviews        []*github.Review        `json:"reviews"`
		ReviewComments []*github.ReviewComment `json:"review_comments"`
	}
	Compare     map[string]string
	Imports     []*github.Import  `json:"imports"`
	Projects    []*github.Project `json:"projects"`
	Hooks       []*github.Hook    `json:"hooks"`
	CreateHooks []*github.Hook    `json:"create_hooks"`
	UpdateHooks []*github.Hook    `json:"update_hooks"`
}

func (r *testRepo) build(t *testing.T, isTarget bool) repo.Repo {
	return repo.New(github.NewMockClient(
		github.MockListMembers(func(path string) github.Members {
			assert.True(t, isTarget)
			return github.MembersFromSlice(r.Members)
		}),
		github.MockGetRepo(func(path string) (*github.Repo, error) {
			return r.Repo, nil
		}),
		github.MockUpdateRepo(func(path string, params *github.UpdateRepoParams) (*github.Repo, error) {
			assert.True(t, isTarget)
			assert.Equal(t, "/repos/"+r.Repo.FullName, path)
			assert.NotNil(t, r.UpdateRepo)
			assert.Equal(t, r.UpdateRepo.Name, params.Name)
			assert.Equal(t, r.UpdateRepo.Description, params.Description)
			assert.Equal(t, r.UpdateRepo.Homepage, params.Homepage)
			assert.Equal(t, r.UpdateRepo.Private, params.Private)
			return r.UpdateRepo, nil
		}),
		github.MockListLabels(func(path string) github.Labels {
			return github.LabelsFromSlice(r.Labels)
		}),
		github.MockCreateLabel((func(i int) func(string, *github.CreateLabelParams) (*github.Label, error) {
			return func(path string, params *github.CreateLabelParams) (*github.Label, error) {
				defer func() { i++ }()
				assert.True(t, isTarget)
				require.Greater(t, len(r.CreateLabels), i)
				assert.Equal(t, "/repos/"+r.Repo.FullName+"/labels", path)
				assert.Equal(t, r.CreateLabels[i].Name, params.Name)
				assert.Equal(t, r.CreateLabels[i].Color, params.Color)
				assert.Equal(t, r.CreateLabels[i].Description, params.Description)
				return nil, nil
			}
		})(0)),
		github.MockUpdateLabel((func(i int) func(string, string, *github.UpdateLabelParams) (*github.Label, error) {
			return func(path, name string, params *github.UpdateLabelParams) (*github.Label, error) {
				defer func() { i++ }()
				assert.True(t, isTarget)
				require.Greater(t, len(r.UpdateLabels), i)
				assert.Equal(t, "/repos/"+r.Repo.FullName+"/labels/"+r.UpdateLabels[i].Name, path)
				assert.Equal(t, r.UpdateLabels[i].Name, name)
				assert.Equal(t, r.UpdateLabels[i].Name, params.Name)
				assert.Equal(t, r.UpdateLabels[i].Color, params.Color)
				assert.Equal(t, r.UpdateLabels[i].Description, params.Description)
				return nil, nil
			}
		})(0)),
		github.MockListIssues(func(path string, _ *github.ListIssuesParams) github.Issues {
			xs := make([]*github.Issue, len(r.Issues))
			for i, s := range r.Issues {
				xs[i] = &s.PullReq.Issue
			}
			return github.IssuesFromSlice(xs)
		}),
		github.MockListComments(func(path string, issueNumber int) github.Comments {
			assert.True(t, !isTarget)
			for _, s := range r.Issues {
				if s.Issue.Number == issueNumber {
					return github.CommentsFromSlice(s.Comments)
				}
			}
			panic(fmt.Sprintf("unexpected issue number: %d", issueNumber))
		}),
		github.MockListEvents(func(path string, issueNumber int) github.Events {
			assert.True(t, !isTarget)
			for _, s := range r.Issues {
				if s.Issue.Number == issueNumber {
					return github.EventsFromSlice(s.Events)
				}
			}
			panic(fmt.Sprintf("unexpected issue number: %d", issueNumber))
		}),
		github.MockGetPullReq(func(path string, pullNumber int) (*github.PullReq, error) {
			assert.True(t, !isTarget)
			for _, s := range r.Issues {
				if s.PullReq.Number == pullNumber {
					return s.PullReq, nil
				}
			}
			panic(fmt.Sprintf("unexpected pull request number: %d", pullNumber))
		}),
		github.MockListPullReqCommits(func(path string, pullNumber int) github.Commits {
			assert.True(t, !isTarget)
			for _, s := range r.Issues {
				if s.PullReq.Number == pullNumber {
					return github.CommitsFromSlice(s.Commits)
				}
			}
			panic(fmt.Sprintf("unexpected pull request number: %d", pullNumber))
		}),
		github.MockGetCompare(func(path string, base, head string) (string, error) {
			assert.True(t, !isTarget)
			if diff, ok := r.Compare[base+"..."+head]; ok {
				return diff, nil
			}
			panic(fmt.Sprintf("unexpected compare: %s...%s", base, head))
		}),
		github.MockListReviews(func(path string, pullNumber int) github.Reviews {
			assert.True(t, !isTarget)
			for _, s := range r.Issues {
				if s.PullReq.Number == pullNumber {
					return github.ReviewsFromSlice(s.Reviews)
				}
			}
			panic(fmt.Sprintf("unexpected pull request number: %d", pullNumber))
		}),
		github.MockListReviewComments(func(path string, pullNumber int) github.ReviewComments {
			assert.True(t, !isTarget)
			for _, s := range r.Issues {
				if s.PullReq.Number == pullNumber {
					return github.ReviewCommentsFromSlice(s.ReviewComments)
				}
			}
			panic(fmt.Sprintf("unexpected pull request number: %d", pullNumber))
		}),
		github.MockGetProject(func(projectID int) (*github.Project, error) {
			assert.True(t, !isTarget)
			for _, p := range r.Projects {
				if p.ID == projectID {
					return p, nil
				}
			}
			panic(fmt.Sprintf("unexpected project id: %d", projectID))
		}),
		github.MockListHooks(func(path string) github.Hooks {
			return github.HooksFromSlice(r.Hooks)
		}),
		github.MockCreateHook((func(i int) func(string, *github.CreateHookParams) (*github.Hook, error) {
			return func(path string, params *github.CreateHookParams) (*github.Hook, error) {
				defer func() { i++ }()
				assert.True(t, isTarget)
				require.Greater(t, len(r.CreateHooks), i)
				assert.Equal(t, "/repos/"+r.Repo.FullName+"/hooks", path)
				assert.Equal(t, r.CreateHooks[i].Events, params.Events)
				assert.Equal(t, r.CreateHooks[i].Config, params.Config)
				assert.Equal(t, r.CreateHooks[i].Active, params.Active)
				return nil, nil
			}
		})(0)),
		github.MockUpdateHook((func(i int) func(string, int, *github.UpdateHookParams) (*github.Hook, error) {
			return func(path string, hookID int, params *github.UpdateHookParams) (*github.Hook, error) {
				defer func() { i++ }()
				assert.True(t, isTarget)
				require.Greater(t, len(r.UpdateHooks), i)
				assert.Equal(t, "/repos/"+r.Repo.FullName+"/hooks/"+fmt.Sprint(hookID), path)
				assert.Equal(t, r.UpdateHooks[i].Events, params.Events)
				assert.Equal(t, r.UpdateHooks[i].Config, params.Config)
				assert.Equal(t, r.UpdateHooks[i].Active, params.Active)
				return nil, nil
			}
		})(0)),
		github.MockImport((func(i int) func(string, *github.Import) (*github.ImportResult, error) {
			return func(path string, x *github.Import) (*github.ImportResult, error) {
				defer func() { i++ }()
				assert.True(t, isTarget)
				require.Greater(t, len(r.Imports), i)
				assert.Equal(t, "/repos/"+r.Repo.FullName+"/import/issues", path)
				assert.Equal(t, r.Imports[i], x)
				return &github.ImportResult{
					ID:     12345,
					Status: "pending",
					URL:    "http://localhost/repo/example/target/import/issues/12345",
				}, nil
			}
		})(0)),
		github.MockGetImport(func(path string, id int) (*github.ImportResult, error) {
			assert.True(t, isTarget)
			assert.Equal(t, 12345, id)
			return &github.ImportResult{
				ID:     12345,
				Status: "imported",
				URL:    "http://localhost/repo/example/target/import/issues/12345",
			}, nil
		}),
	), r.Repo.FullName)
}

func TestMigratorMigrate(t *testing.T) {
	f, err := os.Open("test.yaml")
	require.NoError(t, err)
	defer f.Close()

	var testCases []struct {
		Name        string            `json:"name"`
		Source      *testRepo         `json:"source"`
		Target      *testRepo         `json:"target"`
		UserMapping map[string]string `json:"user_mapping"`
	}
	require.NoError(t, decodeYAML(f, &testCases))

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			source := tc.Source.build(t, false)
			target := tc.Target.build(t, true)
			migrator := New(source, target, tc.UserMapping)
			assert.Nil(t, migrator.Migrate())
		})
	}
}

func decodeYAML(r io.Reader, d interface{}) error {
	// decode to interface once to use json tags
	var m interface{}
	if err := yaml.NewDecoder(r).Decode(&m); err != nil {
		return err
	}
	bs, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(bs, d)
}
