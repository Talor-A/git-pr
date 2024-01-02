package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

type NewPRBody struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Head  string `json:"head"`
	Base  string `json:"base"`
}

func githubGetPRNumberForCommit(commit, prev *Commit) (int, error) {
	type PR struct {
		Number int `json:"number"`
		Head   struct {
			Ref string `json:"ref"`
		} `json:"head"`
		UpdatedAt *time.Time
	}
	if commit.PRNumber != 0 {
		return commit.PRNumber, nil
	}
	ghURL := fmt.Sprintf("https://api.%v/repos/%v/commits/%v/pulls?per_page=100", config.Host, config.Repo, commit.Hash)
	jsonBody, err := httpGET(ghURL)
	if err != nil && strings.Contains(err.Error(), "No commit found") {
		return githubSearchPRNumberForCommit(commit)
	}
	if err != nil {
		return 0, err
	}
	if err == nil {
		var out []PR
		err = json.Unmarshal(jsonBody, &out)
		if err != nil {
			return 0, errorf("failed to parse request body: %v", err)
		}

		remoteRef := commit.GetRemoteRef()
		if remoteRef != "" {
			for _, pr := range out {
				if pr.Head.Ref == remoteRef {
					return pr.Number, nil
				}
			}
		}
		if commit.Skip {
			return githubSearchPRNumberForCommit(commit)
		}
	}

	// The commit was pushed and got "Everything up-to-date", try creating new pr
	err = githubCreatePRForCommit(commit, prev)
	if err != nil {
		return 0, err
	}
	return commit.PRNumber, nil
}

func githubCreatePRForCommit(commit *Commit, prev *Commit) error {
	// attempt to create new PR
	ghURL := fmt.Sprintf("https://api.%v/repos/%v/pulls", config.Host, config.Repo)
	body := NewPRBody{
		Title: commit.Title,
		Body:  commit.Message,
		Head:  commit.GetAttr(KeyRemoteRef),
		Base:  xif(prev != nil, prev.GetRemoteRef(), config.MainBranch),
	}
	fmt.Printf("create pull request for %q\n", commit.Title)
	jsonBody := must(httpPOST(ghURL, body))
	number := gjson.GetBytes(jsonBody, "number").Int()
	if number == 0 {
		return errorf("unexpected")
	}
	commit.PRNumber = int(number)
	time.Sleep(1 * time.Second)
	return nil
}

func githubPRUpdateBaseForCommit(commit *Commit, prev *Commit) error {
	base := xif(prev != nil, prev.GetRemoteRef(), config.MainBranch)
	prNumber := must(githubGetPRNumberForCommit(commit, prev))
	_, err := execGh("pr", "edit", strconv.Itoa(prNumber), "--base", base)
	return err
}

var regexpNumber = regexp.MustCompile(`[0-9]+`)

func githubSearchPRNumberForCommit(commit *Commit) (int, error) {
	query := fmt.Sprintf("in:title %v", commit.Title)
	result, err := execGh("pr", "list", "--limit=1", "--search", query)
	if err != nil {
		debugf("failed to search PR for commit (ignored) %q: %v\n", commit.Title, err)
		return 0, nil
	}
	s := regexpNumber.FindString(result)
	if s == "" {
		return 0, nil
	}
	return must(strconv.Atoi(s)), nil
}
