package main

import (
	"fmt"
	"html/template"
	html "html/template"
	"os"
	"path/filepath"
	"strings"

	git "github.com/gogs/git-module"
	"github.com/picosh/pico/pastes"
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
	Desc       string
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
	Readme   template.HTML
	Rev      *git.Reference
	RevName  string
}

type CommitPageData struct {
	CommitMsg template.HTML
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

func CommitURL(repo string, commitID string) string {
	return fmt.Sprintf("/%s/commits/%s.html", repo, commitID)
}

func repoName(root string) string {
	_, file := filepath.Split(root)
	return file
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
	fmt.Println(dir)
	fmt.Println(data.Name)
	err = os.MkdirAll(dir, os.ModePerm)
	bail(err)

	w, err := os.OpenFile(filepath.Join(dir, data.Name), os.O_WRONLY|os.O_CREATE, 0755)
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
	fmt.Println(dir)
	err = os.MkdirAll(dir, os.ModePerm)
	bail(err)

	w, err := os.OpenFile(filepath.Join(dir, "index.html"), os.O_WRONLY|os.O_CREATE, 0755)
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

type FileData struct {
	Contents template.HTML
}

func writeHTMLTreeFiles(data *PageData) string {
	readme := ""
	for _, file := range data.Tree {
		b, err := file.Entry.Blob().Bytes()
		bail(err)
		file.NumLines = len(strings.Split(string(b), "\n"))

		d := filepath.Dir(file.Path)
		contents, err := pastes.ParseText(file.Entry.Name(), string(b))
		bail(err)

		nameLower := strings.ToLower(file.Entry.Name())
		if nameLower == "readme.md" {
			readme = contents
		}

		writeHtml(&WriteData{
			Name:     fmt.Sprintf("%s.html", file.Entry.Name()),
			Template: "./html/file.page.tmpl",
			Data:     &FileData{Contents: template.HTML(contents)},
			RepoName: data.Repo.Name,
			Subdir:   filepath.Join("tree", data.RevName, "item", d),
			Repo:     data.Repo,
		})
	}
	return readme
}

func writeLogDiffs(project string, repo *git.Repository, data *PageData, cache map[string]bool) {
	for _, commit := range data.Log {
		commitID := commit.ID.String()

		if cache[commitID] {
			continue
		} else {
			cache[commitID] = true
		}

		ancestors, err := commit.Ancestors()
		bail(err)

		// if no ancestors exist then we are at initial commit
		parent := commit
		if len(ancestors) > 0 {
			pt := ancestors[0]
			parent = &CommitData{
				Commit: pt,
				URL:    CommitURL(project, pt.ID.String()),
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
			finContent, err := pastes.ParseText("commit.diff", content)
			bail(err)

			fl.Content = template.HTML(finContent)
			fls = append(fls, fl)
		}
		rnd.Files = fls

		commitData := &CommitPageData{
			Commit:    commit,
			Diff:      rnd,
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
	}
}

func writeRepo(config *RepoConfig) {
	repo, err := git.Open(config.Path)
	bail(err)

	name := repoName(config.Path)
	desc := config.Desc

	heads, err := repo.ShowRef(git.ShowRefOptions{Heads: true, Tags: false})
	bail(err)

	rev := findDefaultBranch(config, heads)
	if rev == nil {
		bail(fmt.Errorf("no default branch found"))
	}
	_, revName := filepath.Split(rev.Refspec)

	repoData := &RepoData{
		Name:       name,
		Desc:       desc,
		SummaryURL: fmt.Sprintf("/%s/index.html", name),
		TreeURL:    fmt.Sprintf("/%s/tree/%s/index.html", name, revName),
		LogURL:     fmt.Sprintf("/%s/logs/%s/index.html", name, revName),
		RefsURL:    fmt.Sprintf("/%s/refs.html", name),
		CloneURL:   fmt.Sprintf("/%s.git", name),
	}

	tags, _ := repo.ShowRef(git.ShowRefOptions{Heads: false, Tags: true})

	cache := make(map[string]bool)

	readme := ""
	for _, revn := range config.Refs {
		for _, head := range heads {
			_, headName := filepath.Split(head.Refspec)
			if revn != headName {
				continue
			}
			data := &PageData{
				Branches: heads,
				Tags:     tags,
				Rev:      head,
				RevName:  headName,
				Repo:     repoData,
			}

			branchReadme := writeBranch(repo, data, cache)
			if readme == "" {
				readme = branchReadme
			}
		}
	}

	data := &PageData{
		Branches: heads,
		Tags:     tags,
		Rev:      rev,
		RevName:  revName,
		Repo:     repoData,
		Readme:   template.HTML(readme),
	}
	writeRefs(data)
	writeRootSummary(data)
}

func writeBranch(repo *git.Repository, pageData *PageData, cache map[string]bool) string {
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

	writeLog(pageData)
	readme := writeHTMLTreeFiles(pageData)
	writeLogDiffs(pageData.Repo.Name, repo, pageData, cache)

	for _, def := range defaultBranches {
		if def == pageData.RevName {
			writeTree(pageData)
		}
	}

	return readme
}

type RepoConfig struct {
	Path string   `mapstructure:"path"`
	Refs []string `mapstructure:"refs"`
	Desc string   `mapstructure:"desc"`
}
type Config struct {
	Repos []*RepoConfig `mapstructure:"repos"`
	URL   string        `mapstructure:"url"`
}

func main() {
	viper.SetDefault("outdir", "./public")
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	err := viper.ReadInConfig()
	bail(err)

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		fmt.Println(err)
		return
	}
	repoList := []*RepoItemData{}
	for _, r := range config.Repos {
		name := repoName(r.Path)
		url := filepath.Join("/", name, "index.html")
		repoList = append(repoList, &RepoItemData{
			URL:  url,
			Name: name,
		})
	}
	writeIndex(&IndexPage{
		RepoList: repoList,
	})
	for _, r := range config.Repos {
		writeRepo(r)
	}
}
