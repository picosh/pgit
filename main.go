package main

import (
	"fmt"
	html "html/template"
	"os"
	"path"
	"strings"

	git "github.com/gogs/git-module"
	"github.com/spf13/viper"
)

var defaultBranches = []string{"main", "master"}

type RepoItemData struct {
	URL  string
	Name string
}

type IndexPage struct {
	RepoList []*RepoItemData
}

type RepoData struct {
	Name       string
	SummaryURL string
	TreeURL    string
	LogURL     string
	RefsURL    string
	CloneURL   string
}

type CommitData struct {
	URL string
	*git.Commit
}

type TreeItem struct {
	NumLines int
	URL      string
	Path     string
	Entry    *git.TreeEntry
}

type PageData struct {
	Repo     *RepoData
	Log      []*CommitData
	Branches []*git.Reference
	Tags     []*git.Reference
	Tree     []*TreeItem
	Readme   string
	Rev      *git.Reference
	RevName  string
}

type CommitPageData struct {
	Commit    *CommitData
	Diff      *git.Diff
	Repo      *RepoData
	Parent    string
	ParentURL string
	CommitURL string
}

type WriteData struct {
	Name     string
	Template string
	Data     interface{}
	RepoName string
	Subdir   string
	Repo     *RepoData
}

func bail(err error) {
	if err != nil {
		panic(err)
	}
}

func CommitURL(repo string, commitID string) string {
	return fmt.Sprintf("/%s/commits/%s.html", repo, commitID)
}

func repoName(root string) string {
	_, file := path.Split(root)
	return file
}
func findDefaultBranch(refs []*git.Reference) *git.Reference {
	for _, def := range defaultBranches {
		for _, ref := range refs {
			if "refs/heads/"+def == ref.Refspec {
				return ref
			}
		}
	}

	return nil
}

func walkTree(tree *git.Tree, branch string, curpath string, aggregate []*TreeItem) []*TreeItem {
	entries, err := tree.Entries()
	bail(err)

	for _, entry := range entries {
		fname := curpath + "/" + entry.Name()
		if entry.IsTree() {
			re, _ := tree.Subtree(entry.Name())
			aggregate = walkTree(re, branch, fname, aggregate)
		}

		if entry.IsBlob() {
			aggregate = append(aggregate, &TreeItem{
				Path:  fname,
				Entry: entry,
				URL:   path.Join("/tree", branch, fname),
			})
		}
	}

	return aggregate
}

func writeHtml(data *WriteData) {
	files := []string{data.Template}
	files = append(
		files,
		"./html/header.partial.tmpl",
		"./html/base.layout.tmpl",
	)

	ts, err := html.ParseFiles(files...)
	bail(err)

	outdir := viper.GetString("outdir")
	dir := path.Join(outdir, data.RepoName, data.Subdir)
	fmt.Println(dir)
	fmt.Println(data.Name)
	err = os.MkdirAll(dir, os.ModePerm)
	bail(err)

	w, err := os.OpenFile(path.Join(dir, data.Name), os.O_WRONLY|os.O_CREATE, 0755)
	bail(err)

	err = ts.Execute(w, data)
	bail(err)
}

func writeIndex(data *IndexPage) {
	files := []string{"./html/index.page.tmpl"}
	files = append(
		files,
		"./html/header.partial.tmpl",
		"./html/base.layout.tmpl",
	)

	ts, err := html.ParseFiles(files...)
	bail(err)

	outdir := viper.GetString("outdir")
	dir := path.Join(outdir)
	fmt.Println(dir)
	err = os.MkdirAll(dir, os.ModePerm)
	bail(err)

	w, err := os.OpenFile(path.Join(dir, "index.html"), os.O_WRONLY|os.O_CREATE, 0755)
	bail(err)

	err = ts.Execute(w, data)
	bail(err)
}
func writeSummary(data *PageData) {
	writeHtml(&WriteData{
		Name:     "index.html",
		Template: "./html/summary.page.tmpl",
		Data:     data,
		RepoName: data.Repo.Name,
		Repo:     data.Repo,
	})
}
func writeTree(data *PageData) {
	writeHtml(&WriteData{
		Name:     "tree.html",
		Template: "./html/tree.page.tmpl",
		Data:     data,
		RepoName: data.Repo.Name,
		Repo:     data.Repo,
	})
}
func writeLog(data *PageData) {
	writeHtml(&WriteData{
		Name:     "log.html",
		Template: "./html/log.page.tmpl",
		Data:     data,
		RepoName: data.Repo.Name,
		Repo:     data.Repo,
	})
}
func writeRefs(data *PageData) {
	writeHtml(&WriteData{
		Name:     "refs.html",
		Template: "./html/refs.page.tmpl",
		Data:     data,
		RepoName: data.Repo.Name,
		Repo:     data.Repo,
	})
}

type FileData struct {
	Contents string
}

func writeHTMLTreeFiles(data *PageData) {
	for _, file := range data.Tree {
		b, err := file.Entry.Blob().Bytes()
		bail(err)
		file.NumLines = len(strings.Split(string(b), "\n"))

		d := path.Dir(file.Path)
		writeHtml(&WriteData{
			Name:     fmt.Sprintf("%s.html", file.Entry.Name()),
			Template: "./html/file.page.tmpl",
			Data:     &FileData{Contents: string(b)},
			RepoName: data.Repo.Name,
			Subdir:   path.Join("tree", data.RevName, d),
			Repo:     data.Repo,
		})
	}
}

func writeLogDiffs(project string, repo *git.Repository, data *PageData, cache map[string]bool) {
	var lastCommit *CommitData
	for _, commit := range data.Log {
		if lastCommit == nil {
			lastCommit = commit
			continue
		}

		commitID := commit.ID.String()

		if cache[commitID] {
			continue
		} else {
			cache[commitID] = true
		}

		diff, err := repo.Diff(
			lastCommit.ID.String(),
			0,
			0,
			0,
			git.DiffOptions{Base: commitID},
		)
		bail(err)

		ancestors, err := commit.Ancestors()
		bail(err)

		parentID := ""
		if len(ancestors) > 0 {
			parent := ancestors[0]
			if parent != nil {
				parentID = parent.ID.String()
			}
		}

		commitData := &CommitPageData{
			Commit:    commit,
			Diff:      diff,
			Repo:      data.Repo,
			Parent:    parentID,
			CommitURL: CommitURL(project, commitID),
			ParentURL: CommitURL(project, parentID),
		}

		writeHtml(&WriteData{
			Name:     fmt.Sprintf("%s.html", commitID),
			Template: "./html/commit.page.tmpl",
			Data:     commitData,
			RepoName: data.Repo.Name,
			Subdir:   "commits",
			Repo:     data.Repo,
		})
		lastCommit = commit
	}
}

func writeRepo(root string) {
	repo, err := git.Open(root)
	bail(err)

	name := repoName(root)
	repoData := &RepoData{
		Name:       name,
		SummaryURL: fmt.Sprintf("/%s/index.html", name),
		TreeURL:    fmt.Sprintf("/%s/tree.html", name),
		LogURL:     fmt.Sprintf("/%s/log.html", name),
		RefsURL:    fmt.Sprintf("/%s/refs.html", name),
		CloneURL:   fmt.Sprintf("/%s.git", name),
	}

	heads, err := repo.ShowRef(git.ShowRefOptions{Heads: true, Tags: false})
	bail(err)

	tags, _ := repo.ShowRef(git.ShowRefOptions{Heads: false, Tags: true})

	cache := make(map[string]bool)
	rev := findDefaultBranch(heads)
	if rev != nil {
		// for _, rev := range heads {
		_, revName := path.Split(rev.Refspec)
		data := &PageData{
			Branches: heads,
			Tags:     tags,
			Rev:      rev,
			RevName:  revName,
			Repo:     repoData,
			Readme:   "",
		}

		writeBranch(repo, data, cache)
	}
}

func writeBranch(repo *git.Repository, pageData *PageData, cache map[string]bool) {
	commits, err := repo.CommitsByPage(pageData.Rev.ID, 0, 100)
	bail(err)

	logs := []*CommitData{}
	for _, commit := range commits {
		logs = append(logs, &CommitData{
			URL:    CommitURL(pageData.Repo.Name, commit.ID.String()),
			Commit: commit,
		})
	}

	tree, err := repo.LsTree(pageData.Rev.ID)
	bail(err)
	entries := []*TreeItem{}
	treeEntries := walkTree(tree, pageData.RevName, "", entries)
	for _, entry := range treeEntries {
		entry.Path = strings.TrimPrefix(entry.Path, "/")
		entry.URL = path.Join(
			"/",
			pageData.Repo.Name,
			"tree",
			pageData.RevName,
			fmt.Sprintf("%s.html", entry.Path),
		)
	}

	pageData.Log = logs
	pageData.Tree = treeEntries

	writeSummary(pageData)
	writeLog(pageData)
	writeRefs(pageData)
	writeHTMLTreeFiles(pageData)
	writeLogDiffs(pageData.Repo.Name, repo, pageData, cache)

	for _, def := range defaultBranches {
		if def == pageData.RevName {
			writeTree(pageData)
		}
	}
}

func main() {
	viper.SetDefault("outdir", "./public")
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	err := viper.ReadInConfig()
	bail(err)

	repos := viper.GetStringSlice("repos")
	repoList := []*RepoItemData{}
	for _, r := range repos {
		name := repoName(r)
		url := path.Join("/", name, "index.html")
		repoList = append(repoList, &RepoItemData{
			URL:  url,
			Name: name,
		})
	}
	writeIndex(&IndexPage{
		RepoList: repoList,
	})
	for _, r := range repos {
		writeRepo(r)
	}
}
