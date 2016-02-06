package github

import (
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/bitrise-io/bitrise-webhooks/bitriseapi"
	"github.com/bitrise-io/go-utils/pointers"
	"github.com/stretchr/testify/require"
)

const (
	sampleCodePushData = `{
  "ref": "refs/heads/master",
  "deleted": false,
  "head_commit": {
    "distinct": true,
    "id": "83b86e5f286f546dc5a4a58db66ceef44460c85e",
    "message": "re-structuring Hook Providers, with added tests"
  }
}`

	samplePullRequestData = `{
  "action": "opened",
  "number": 12,
  "pull_request": {
    "head": {
      "ref": "master",
      "sha": "83b86e5f286f546dc5a4a58db66ceef44460c85e"
    },
    "title": "PR test",
    "body": "PR text body",
    "merged": false,
    "mergeable": true
  }
}`
)

func Test_HookProvider_HookCheck(t *testing.T) {
	provider := HookProvider{}

	t.Log("Push event - should handle")
	{
		header := http.Header{
			"X-Github-Event": {"push"},
			"Content-Type":   {"application/json"},
		}
		hookCheckResult := provider.HookCheck(header)
		require.True(t, hookCheckResult.IsSupportedByProvider)
		require.NoError(t, hookCheckResult.CantTransformReason)
	}

	t.Log("Pull Request event - should handle")
	{
		header := http.Header{
			"X-Github-Event": {"pull_request"},
			"Content-Type":   {"application/json"},
		}
		hookCheckResult := provider.HookCheck(header)
		require.True(t, hookCheckResult.IsSupportedByProvider)
		require.NoError(t, hookCheckResult.CantTransformReason)
	}

	t.Log("Ping event (unsupported GH event) - should not transform, should skip")
	{
		header := http.Header{
			"X-Github-Event": {"ping"},
			"Content-Type":   {"application/json"},
		}
		hookCheckResult := provider.HookCheck(header)
		require.True(t, hookCheckResult.IsSupportedByProvider)
		require.EqualError(t, hookCheckResult.CantTransformReason, "Unsupported GitHub hook event type: ping")
	}

	t.Log("Not a GitHub style webhook")
	{
		header := http.Header{
			"Content-Type": {"application/json"},
		}
		hookCheckResult := provider.HookCheck(header)
		require.False(t, hookCheckResult.IsSupportedByProvider)
		require.NoError(t, hookCheckResult.CantTransformReason)
	}

	t.Log("Missing Content-Type")
	{
		header := http.Header{
			"X-Github-Event": {"push"},
		}
		hookCheckResult := provider.HookCheck(header)
		require.False(t, hookCheckResult.IsSupportedByProvider)
		require.NoError(t, hookCheckResult.CantTransformReason)
	}
}

func Test_HookProvider_Transform(t *testing.T) {
	provider := HookProvider{}

	t.Log("Code Push - should not be skipped")
	{
		request := http.Request{
			Header: http.Header{"X-Github-Event": {"push"}},
			Body:   ioutil.NopCloser(strings.NewReader("hi")),
		}
		hookTransformResult := provider.Transform(&request)
		require.False(t, hookTransformResult.ShouldSkip)
	}

	t.Log("Pull Request - should not be skipped")
	{
		request := http.Request{
			Header: http.Header{"X-Github-Event": {"pull_request"}},
			Body:   ioutil.NopCloser(strings.NewReader("hi")),
		}
		hookTransformResult := provider.Transform(&request)
		require.False(t, hookTransformResult.ShouldSkip)
	}

	t.Log("No Request Body")
	{
		request := http.Request{
			Header: http.Header{"X-Github-Event": {"push"}},
		}
		hookTransformResult := provider.Transform(&request)
		require.False(t, hookTransformResult.ShouldSkip)
		require.EqualError(t, hookTransformResult.Error, "Failed to read content of request body: no or empty request body")
	}

	t.Log("Code Push - should be handled")
	{
		request := http.Request{
			Header: http.Header{
				"X-Github-Event": {"push"},
				"Content-Type":   {"application/json"},
			},
			Body: ioutil.NopCloser(strings.NewReader(sampleCodePushData)),
		}
		hookTransformResult := provider.Transform(&request)
		require.NoError(t, hookTransformResult.Error)
		require.False(t, hookTransformResult.ShouldSkip)
		require.Equal(t, bitriseapi.TriggerAPIParamsModel{
			CommitHash:    "83b86e5f286f546dc5a4a58db66ceef44460c85e",
			CommitMessage: "re-structuring Hook Providers, with added tests",
			Branch:        "master",
		}, hookTransformResult.TriggerAPIParams)
	}

	t.Log("Pull Request - should be handled")
	{
		request := http.Request{
			Header: http.Header{
				"X-Github-Event": {"pull_request"},
				"Content-Type":   {"application/json"},
			},
			Body: ioutil.NopCloser(strings.NewReader(samplePullRequestData)),
		}
		hookTransformResult := provider.Transform(&request)
		require.NoError(t, hookTransformResult.Error)
		require.False(t, hookTransformResult.ShouldSkip)
		require.Equal(t, bitriseapi.TriggerAPIParamsModel{
			CommitHash:    "83b86e5f286f546dc5a4a58db66ceef44460c85e",
			CommitMessage: "PR test\n\nPR text body",
			Branch:        "master",
			PullRequestID: pointers.NewIntPtr(12),
		}, hookTransformResult.TriggerAPIParams)
	}
}

func Test_transformCodePushEvent(t *testing.T) {
	t.Log("Not Distinct Head Commit")
	{
		codePush := CodePushEventModel{
			HeadCommit: CommitModel{Distinct: false},
		}
		hookTransformResult := transformCodePushEvent(codePush)
		require.True(t, hookTransformResult.ShouldSkip)
		require.EqualError(t, hookTransformResult.Error, "Head Commit is not Distinct")
	}

	t.Log("This is a 'deleted' event")
	{
		codePush := CodePushEventModel{
			HeadCommit: CommitModel{
				Distinct: true,
			},
			Deleted: true,
		}
		hookTransformResult := transformCodePushEvent(codePush)
		require.True(t, hookTransformResult.ShouldSkip)
		require.EqualError(t, hookTransformResult.Error, "This is a 'Deleted' event, no build can be started")
	}

	t.Log("Not a head ref")
	{
		codePush := CodePushEventModel{
			Ref:        "refs/pull/a",
			HeadCommit: CommitModel{Distinct: true},
		}
		hookTransformResult := transformCodePushEvent(codePush)
		require.True(t, hookTransformResult.ShouldSkip)
		require.EqualError(t, hookTransformResult.Error, "Ref (refs/pull/a) is not a head ref")
	}

	t.Log("Do Transform")
	{
		codePush := CodePushEventModel{
			Ref: "refs/heads/master",
			HeadCommit: CommitModel{
				Distinct:      true,
				CommitHash:    "83b86e5f286f546dc5a4a58db66ceef44460c85e",
				CommitMessage: "re-structuring Hook Providers, with added tests",
			},
		}
		hookTransformResult := transformCodePushEvent(codePush)
		require.NoError(t, hookTransformResult.Error)
		require.False(t, hookTransformResult.ShouldSkip)
		require.Equal(t, bitriseapi.TriggerAPIParamsModel{
			CommitHash:    "83b86e5f286f546dc5a4a58db66ceef44460c85e",
			CommitMessage: "re-structuring Hook Providers, with added tests",
			Branch:        "master",
		}, hookTransformResult.TriggerAPIParams)
	}
}

func Test_transformPullRequestEvent(t *testing.T) {
	t.Log("Unsupported Pull Request action")
	{
		pullRequest := PullRequestEventModel{
			Action: "labeled",
		}
		hookTransformResult := transformPullRequestEvent(pullRequest)
		require.True(t, hookTransformResult.ShouldSkip)
		require.EqualError(t, hookTransformResult.Error, "Pull Request action doesn't require a build: labeled")
	}

	t.Log("Empty Pull Request action")
	{
		pullRequest := PullRequestEventModel{}
		hookTransformResult := transformPullRequestEvent(pullRequest)
		require.True(t, hookTransformResult.ShouldSkip)
		require.EqualError(t, hookTransformResult.Error, "No Pull Request action specified")
	}

	t.Log("Already Merged")
	{
		pullRequest := PullRequestEventModel{
			Action: "opened",
			PullRequestInfo: PullRequestInfoModel{
				Merged: true,
			},
		}
		hookTransformResult := transformPullRequestEvent(pullRequest)
		require.True(t, hookTransformResult.ShouldSkip)
		require.EqualError(t, hookTransformResult.Error, "Pull Request already merged")
	}

	t.Log("Not Mergeable")
	{
		pullRequest := PullRequestEventModel{
			Action: "reopened",
			PullRequestInfo: PullRequestInfoModel{
				Merged:    false,
				Mergeable: pointers.NewBoolPtr(false),
			},
		}
		hookTransformResult := transformPullRequestEvent(pullRequest)
		require.True(t, hookTransformResult.ShouldSkip)
		require.EqualError(t, hookTransformResult.Error, "Pull Request is not mergeable")
	}

	t.Log("Mergeable: not yet decided")
	{
		pullRequest := PullRequestEventModel{
			Action:        "synchronize",
			PullRequestID: 12,
			PullRequestInfo: PullRequestInfoModel{
				Title:     "PR test",
				Merged:    false,
				Mergeable: nil,
				BranchInfo: BranchInfoModel{
					Ref:        "master",
					CommitHash: "83b86e5f286f546dc5a4a58db66ceef44460c85e",
				},
			},
		}
		hookTransformResult := transformPullRequestEvent(pullRequest)
		require.False(t, hookTransformResult.ShouldSkip)
		require.NoError(t, hookTransformResult.Error)
		require.Equal(t, bitriseapi.TriggerAPIParamsModel{
			CommitHash:    "83b86e5f286f546dc5a4a58db66ceef44460c85e",
			CommitMessage: "PR test",
			Branch:        "master",
			PullRequestID: pointers.NewIntPtr(12),
		}, hookTransformResult.TriggerAPIParams)
	}

	t.Log("Mergeable: true")
	{
		pullRequest := PullRequestEventModel{
			Action:        "synchronize",
			PullRequestID: 12,
			PullRequestInfo: PullRequestInfoModel{
				Title:     "PR test",
				Merged:    false,
				Mergeable: pointers.NewBoolPtr(true),
				BranchInfo: BranchInfoModel{
					Ref:        "master",
					CommitHash: "83b86e5f286f546dc5a4a58db66ceef44460c85e",
				},
			},
		}
		hookTransformResult := transformPullRequestEvent(pullRequest)
		require.NoError(t, hookTransformResult.Error)
		require.False(t, hookTransformResult.ShouldSkip)
		require.Equal(t, bitriseapi.TriggerAPIParamsModel{
			CommitHash:    "83b86e5f286f546dc5a4a58db66ceef44460c85e",
			CommitMessage: "PR test",
			Branch:        "master",
			PullRequestID: pointers.NewIntPtr(12),
		}, hookTransformResult.TriggerAPIParams)
	}

	t.Log("Pull Request - Title & Body")
	{
		pullRequest := PullRequestEventModel{
			Action:        "opened",
			PullRequestID: 12,
			PullRequestInfo: PullRequestInfoModel{
				Title:     "PR test",
				Body:      "PR text body",
				Merged:    false,
				Mergeable: pointers.NewBoolPtr(true),
				BranchInfo: BranchInfoModel{
					Ref:        "master",
					CommitHash: "83b86e5f286f546dc5a4a58db66ceef44460c85e",
				},
			},
		}
		hookTransformResult := transformPullRequestEvent(pullRequest)
		require.NoError(t, hookTransformResult.Error)
		require.False(t, hookTransformResult.ShouldSkip)
		require.Equal(t, bitriseapi.TriggerAPIParamsModel{
			CommitHash:    "83b86e5f286f546dc5a4a58db66ceef44460c85e",
			CommitMessage: "PR test\n\nPR text body",
			Branch:        "master",
			PullRequestID: pointers.NewIntPtr(12),
		}, hookTransformResult.TriggerAPIParams)
	}
}