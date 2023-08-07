package main

import (
	"fmt"
	"html/template"
	html "html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dustin/go-humanize"
	git "github.com/gogs/git-module"
	"github.com/mergestat/timediff"
	"github.com/picosh/pico/pastes"
	"github.com/picosh/pico/shared"
	"github.com/spf13/viper"
)

var defaultBranches = []string{"main", "master"}

type RepoItemData struct {
	URL        string
	Name       string
	Desc       string
	CommitDate string
	LastCommit *git.Commit
}

type IndexPage struct {
	RepoList []*RepoItemData
}

type RepoData struct {
	Name               string
	Desc               string
	SummaryURL         string
	TreeURL            string
	LogURL             string
	RefsURL            string
	CloneURL           string
	MaxCommits         int
	RevName            string
	Readme             string
	HideTreeLastCommit bool
}

type CommitData struct {
	SummaryStr string
	URL        string
	WhenStr    string
	AuthorStr  string
	ShortID    string
	*git.Commit
}

type TreeItem struct {
	IsTextFile bool
	Size       string
	NumLines   int
	URL        string
	Path       string
	Entry      *git.TreeEntry
	CommitURL  string
	Summary    string
	When       string
}

type PageData struct {
	Repo    *RepoData
	Log     []*CommitData
	Tree    []*TreeItem
	Readme  template.HTML
	Rev     *git.Reference
	RevName string
	Refs    []*RefInfo
}

type CommitPageData struct {
	CommitMsg template.HTML
	CommitID  string
	Commit    *CommitData
	Diff      *DiffRender
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

type DiffRender struct {
	NumFiles       int
	TotalAdditions int
	TotalDeletions int
	Files          []*DiffRenderFile
}

type DiffRenderFile struct {
	FileType     string
	OldMode      git.EntryMode
	OldName      string
	Mode         git.EntryMode
	Name         string
	Content      template.HTML
	NumAdditions int
	NumDeletions int
}

type FileData struct {
	Contents template.HTML
	Name     string
}

type RefInfo struct {
	Refspec string
	URL     template.URL
}

type BranchOutput struct {
	Readme     string
	LastCommit *git.Commit
}

type RepoConfig struct {
	Path               string   `mapstructure:"path"`
	Refs               []string `mapstructure:"refs"`
	Desc               string   `mapstructure:"desc"`
	MaxCommits         int      `mapstructure:"max_commits"`
	Readme             string   `mapstructure:"readme"`
	HideTreeLastCommit bool     `mapstructure:"hide_tree_last_commit"`
}

type Config struct {
	Repos []*RepoConfig `mapstructure:"repos"`
	URL   string        `mapstructure:"url"`
	Cache map[string]bool
}

func diffFileType(_type git.DiffFileType) string {
	if _type == git.DiffFileAdd {
		return "A"
	} else if _type == git.DiffFileChange {
		return "M"
	} else if _type == git.DiffFileDelete {
		return "D"
	} else if _type == git.DiffFileRename {
		return "R"
	}

	return ""
}

func bail(err error) {
	if err != nil {
		panic(err)
	}
}

func toPretty(b int64) string {
	return humanize.Bytes(uint64(b))
}

func commitURL(repo string, commitID string) string {
	return fmt.Sprintf("/%s/commits/%s.html", repo, commitID)
}

func repoName(root string) string {
	_, file := filepath.Split(root)
	return file
}

func readmeFile(repo *RepoData) string {
	if repo.Readme == "" {
		return "readme.md"
	}

	return strings.ToLower(repo.Readme)
}

func findDefaultBranch(config *RepoConfig, refs []*git.Reference) *git.Reference {
	branches := config.Refs
	if len(branches) == 0 {
		branches = defaultBranches
	}

	for _, def := range branches {
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
				Size:  toPretty(entry.Size()),
				Path:  fname,
				Entry: entry,
				URL:   filepath.Join("/", "tree", branch, "item", fname),
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
	dir := filepath.Join(outdir, data.RepoName, data.Subdir)
	err = os.MkdirAll(dir, os.ModePerm)
	bail(err)

	fp := filepath.Join(dir, data.Name)
	fmt.Printf("writing (%s)\n", fp)

	w, err := os.OpenFile(fp, os.O_WRONLY|os.O_CREATE, 0755)
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
	dir := filepath.Join(outdir)
	err = os.MkdirAll(dir, os.ModePerm)
	bail(err)

	fp := filepath.Join(dir, "index.html")
	fmt.Printf("writing (%s)\n", fp)

	w, err := os.OpenFile(fp, os.O_WRONLY|os.O_CREATE, 0755)
	bail(err)

	err = ts.Execute(w, data)
	bail(err)
}

func writeRootSummary(data *PageData) {
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
		Name:     "index.html",
		Subdir:   filepath.Join("tree", data.RevName),
		Template: "./html/tree.page.tmpl",
		Data:     data,
		RepoName: data.Repo.Name,
		Repo:     data.Repo,
	})
}
func writeLog(data *PageData) {
	writeHtml(&WriteData{
		Name:     "index.html",
		Subdir:   filepath.Join("logs", data.RevName),
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

func writeHTMLTreeFiles(data *PageData) string {
	readme := ""
	for _, file := range data.Tree {
		b, err := file.Entry.Blob().Bytes()
		bail(err)
		str := string(b)

		file.IsTextFile = shared.IsTextFile(str)

		if file.IsTextFile {
			file.NumLines = len(strings.Split(str, "\n"))
		}

		d := filepath.Dir(file.Path)
		contents, err := pastes.ParseText(file.Entry.Name(), string(b))
		bail(err)

		nameLower := strings.ToLower(file.Entry.Name())
		summary := readmeFile(data.Repo)
		if nameLower == summary {
			readme = contents
		}

		writeHtml(&WriteData{
			Name:     fmt.Sprintf("%s.html", file.Entry.Name()),
			Template: "./html/file.page.tmpl",
			Data: &FileData{
				Contents: template.HTML(contents),
				Name:     file.Entry.Name(),
			},
			RepoName: data.Repo.Name,
			Subdir:   filepath.Join("tree", data.RevName, "item", d),
			Repo:     data.Repo,
		})
	}
	return readme
}

func (c *Config) writeLogDiffs(repo *git.Repository, pageData *PageData) {
	project := pageData.Repo.Name
	for _, commit := range pageData.Log {
		commitID := commit.ID.String()

		if c.Cache[commitID] {
			continue
		} else {
			c.Cache[commitID] = true
		}

		ancestors, err := commit.Ancestors()
		bail(err)

		// if no ancestors exist then we are at initial commit
		parent := commit
		if len(ancestors) > 0 {
			pt := ancestors[0]
			parent = &CommitData{
				Commit: pt,
				URL:    commitURL(project, pt.ID.String()),
			}
		}
		parentID := parent.ID.String()

		diff, err := repo.Diff(
			parentID,
			0,
			0,
			0,
			git.DiffOptions{Base: commitID},
		)

		rnd := &DiffRender{
			NumFiles:       diff.NumFiles(),
			TotalAdditions: diff.TotalAdditions(),
			TotalDeletions: diff.TotalDeletions(),
		}
		fls := []*DiffRenderFile{}
		for _, file := range diff.Files {
			fl := &DiffRenderFile{
				FileType:     diffFileType(file.Type),
				OldMode:      file.OldMode(),
				OldName:      file.OldName(),
				Mode:         file.Mode(),
				Name:         file.Name,
				NumAdditions: file.NumAdditions(),
				NumDeletions: file.NumDeletions(),
			}
			content := ""
			for _, section := range file.Sections {
				for _, line := range section.Lines {
					content += fmt.Sprintf("%s\n", line.Content)
				}
			}
			// set filename to something our `ParseText` recognizes (e.g. `.diff`)
			finContent, err := pastes.ParseText("commit.diff", content)
			bail(err)

			fl.Content = template.HTML(finContent)
			fls = append(fls, fl)
		}
		rnd.Files = fls

		commitData := &CommitPageData{
			Commit:    commit,
			CommitID:  commit.ID.String()[:7],
			Diff:      rnd,
			Repo:      pageData.Repo,
			Parent:    parentID[:7],
			CommitURL: commitURL(project, commitID),
			ParentURL: commitURL(project, parentID),
		}

		writeHtml(&WriteData{
			Name:     fmt.Sprintf("%s.html", commitID),
			Template: "./html/commit.page.tmpl",
			Data:     commitData,
			RepoName: pageData.Repo.Name,
			Subdir:   "commits",
			Repo:     pageData.Repo,
		})
	}
}

func (c *Config) writeRepo(config *RepoConfig) *BranchOutput {
	repo, err := git.Open(config.Path)
	bail(err)

	name := repoName(config.Path)

	refs, err := repo.ShowRef(git.ShowRefOptions{Heads: true, Tags: true})
	bail(err)

	rev := findDefaultBranch(config, refs)
	if rev == nil {
		bail(fmt.Errorf("no default branch found"))
	}
	_, revName := filepath.Split(rev.Refspec)

	refInfoMap := map[string]*RefInfo{}
	var mainOutput *BranchOutput
	claimed := false
	for _, revn := range config.Refs {
		for _, head := range refs {
			_, headName := filepath.Split(head.Refspec)
			if revn != headName {
				continue
			}
			refInfoMap[head.ID] = &RefInfo{
				Refspec: strings.TrimPrefix(head.Refspec, "refs/"),
				URL:     template.URL(fmt.Sprintf("/%s/tree/%s/index.html", name, revn)),
			}

			branchRepo := &RepoData{
				Name:               name,
				Desc:               config.Desc,
				MaxCommits:         config.MaxCommits,
				Readme:             config.Readme,
				HideTreeLastCommit: config.HideTreeLastCommit,
				SummaryURL:         fmt.Sprintf("/%s/index.html", name),
				TreeURL:            fmt.Sprintf("/%s/tree/%s/index.html", name, revn),
				LogURL:             fmt.Sprintf("/%s/logs/%s/index.html", name, revn),
				RefsURL:            fmt.Sprintf("/%s/refs.html", name),
				CloneURL:           fmt.Sprintf("https://%s/%s.git", c.URL, name),
				RevName:            revn,
			}

			data := &PageData{
				Rev:     head,
				RevName: headName,
				Repo:    branchRepo,
			}

			branchOutput := c.writeBranch(repo, data)
			if !claimed {
				mainOutput = branchOutput
				claimed = true
			}
		}
	}

	for _, ref := range refs {
		if refInfoMap[ref.ID] != nil {
			continue
		}

		refInfoMap[ref.ID] = &RefInfo{
			Refspec: strings.TrimPrefix(ref.Refspec, "refs/"),
		}
	}

	refInfoList := []*RefInfo{}
	for _, val := range refInfoMap {
		refInfoList = append(refInfoList, val)
	}
	sort.Slice(refInfoList, func(i, j int) bool {
		urlI := refInfoList[i].URL
		urlJ := refInfoList[j].URL
		refI := refInfoList[i].Refspec
		refJ := refInfoList[j].Refspec
		if urlI == urlJ {
			return refI < refJ
		}
		return urlI > urlJ
	})

	repoData := &RepoData{
		Name:       name,
		Desc:       config.Desc,
		SummaryURL: fmt.Sprintf("/%s/index.html", name),
		TreeURL:    fmt.Sprintf("/%s/tree/%s/index.html", name, revName),
		LogURL:     fmt.Sprintf("/%s/logs/%s/index.html", name, revName),
		RefsURL:    fmt.Sprintf("/%s/refs.html", name),
		CloneURL:   fmt.Sprintf("https://%s/%s.git", c.URL, name),
		RevName:    revName,
	}

	data := &PageData{
		Rev:     rev,
		RevName: revName,
		Repo:    repoData,
		Readme:  template.HTML(mainOutput.Readme),
		Refs:    refInfoList,
	}
	writeRefs(data)
	writeRootSummary(data)
	return mainOutput
}

func (c *Config) writeBranch(repo *git.Repository, pageData *PageData) *BranchOutput {
	fmt.Printf("compiling (%s) branch (%s)\n", pageData.Repo.Name, pageData.Repo.RevName)

	output := &BranchOutput{}
	pageSize := pageData.Repo.MaxCommits
	if pageSize == 0 {
		pageSize = 5000
	}

	commits, err := repo.CommitsByPage(pageData.Rev.ID, 0, pageSize)
	bail(err)

	logs := []*CommitData{}
	for i, commit := range commits {
		if i == 0 {
			output.LastCommit = commit
		}

		logs = append(logs, &CommitData{
			URL:        commitURL(pageData.Repo.Name, commit.ID.String()),
			ShortID:    commit.ID.String()[:7],
			SummaryStr: commit.Summary(),
			AuthorStr:  commit.Author.Name,
			WhenStr:    timediff.TimeDiff(commit.Author.When),
			Commit:     commit,
		})
	}

	tree, err := repo.LsTree(pageData.Rev.ID)
	bail(err)
	entries := []*TreeItem{}
	treeEntries := walkTree(tree, pageData.RevName, "", entries)
	for _, entry := range treeEntries {
		entry.Path = strings.TrimPrefix(entry.Path, "/")

		var lastCommits []*git.Commit
		// `git rev-list` is pretty expensive here, so we have a flag to disable
		if pageData.Repo.HideTreeLastCommit {
			fmt.Println("skipping finding last commit for each file")
		} else {
			lastCommits, err = repo.RevList([]string{pageData.Rev.Refspec}, git.RevListOptions{
				Path:           entry.Path,
				CommandOptions: git.CommandOptions{Args: []string{"-1"}},
			})
			bail(err)

			var lc *git.Commit
			if len(lastCommits) > 0 {
				lc = lastCommits[0]
			}
			entry.CommitURL = commitURL(pageData.Repo.Name, lc.ID.String())
			entry.Summary = lc.Summary()
			entry.When = timediff.TimeDiff(lc.Author.When)
		}
		entry.URL = filepath.Join(
			"/",
			pageData.Repo.Name,
			"tree",
			pageData.RevName,
			"item",
			fmt.Sprintf("%s.html", entry.Path),
		)
	}

	pageData.Log = logs
	pageData.Tree = treeEntries

	fmt.Printf("compilation complete (%s) branch (%s)", pageData.Repo.Name, pageData.Repo.RevName)

	writeLog(pageData)
	readme := writeHTMLTreeFiles(pageData)
	c.writeLogDiffs(repo, pageData)

	writeTree(pageData)

	output.Readme = readme
	return output
}

func main() {
	viper.SetDefault("outdir", "./public")
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	err := viper.ReadInConfig()
	bail(err)

	var config Config
	err = viper.Unmarshal(&config)
	bail(err)

	config.Cache = make(map[string]bool)

	repoList := []*RepoItemData{}
	for _, r := range config.Repos {
		mainOutput := config.writeRepo(r)
		name := repoName(r.Path)
		url := filepath.Join("/", name, "index.html")
		repoList = append(repoList, &RepoItemData{
			URL:        url,
			Name:       name,
			Desc:       r.Desc,
			CommitDate: timediff.TimeDiff(mainOutput.LastCommit.Author.When),
			LastCommit: mainOutput.LastCommit,
		})
	}
	sort.Slice(repoList, func(i, j int) bool {
		first := repoList[i].LastCommit.Author.When
		second := repoList[j].LastCommit.Author.When
		return first.After(second)
	})

	writeIndex(&IndexPage{
		RepoList: repoList,
	})
}
