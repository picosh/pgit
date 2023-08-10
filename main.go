package main

import (
	"bytes"
	"embed"
	_ "embed"
	"flag"
	"fmt"
	"html/template"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/alecthomas/chroma"
	formatterHtml "github.com/alecthomas/chroma/formatters/html"
	"github.com/alecthomas/chroma/lexers"
	"github.com/alecthomas/chroma/styles"
	"github.com/dustin/go-humanize"
	git "github.com/gogs/git-module"
	"github.com/mergestat/timediff"
	"go.uber.org/zap"
)

//go:embed html/*.tmpl
var efs embed.FS

type Config struct {
	// required params
	Outdir string
	// abs path to git repo
	RepoPath string

	// optional params
	// generate logs anad tree based on the git revisions provided
	Revs []string
	// description of repo used in the header of site
	Desc string
	// maximum number of commits that we will process in descending order
	MaxCommits int
	// name of the readme file
	Readme string
	// In order to get the latest commit per file we do a `git rev-list {ref} {file}`
	// which is n+1 where n is a file in the tree.
	// We offer a way to disable showing the latest commit in the output
	// for those who want a faster build time
	HideTreeLastCommit bool

	// user-defined urls
	HomeUrl  template.URL
	CloneURL template.URL

	// computed
	// cache for skipping commits, trees, etc.
	Cache map[string]bool
	// pretty name for the repo
	RepoName string
	// logger
	Logger *zap.SugaredLogger
	// chroma style
	Theme *chroma.Style
}

// revision data
type RevData struct {
	ID      string
	RevName string
	TreeURL template.URL
	LogURL  template.URL
}

type TagData struct {
	Name string
	URL template.URL
}

type CommitData struct {
	SummaryStr string
	URL        template.URL
	WhenStr    string
	AuthorStr  string
	ShortID    string
	Refs []*RefInfo
	*git.Commit
}

type TreeItem struct {
	IsTextFile bool
	Size       string
	NumLines   int
	Path       string
	URL        template.URL
	CommitURL  template.URL
	Summary    string
	When       string
	Entry      *git.TreeEntry
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

type RefInfo struct {
	ID string
	Refspec string
	URL     template.URL
}

type BranchOutput struct {
	Readme     string
	LastCommit *git.Commit
}

type SiteURLs struct {
	RootURL    template.URL
	CloneURL   template.URL
	SummaryURL template.URL
	RefsURL    template.URL
}

type PageData struct {
	Repo     *Config
	SiteURLs *SiteURLs
	RevData  *RevData
}

type SummaryPageData struct {
	*PageData
	Readme template.HTML
}

type TreePageData struct {
	*PageData
	Tree []*TreeItem
}

type LogPageData struct {
	*PageData
	Logs []*CommitData
}

type FilePageData struct {
	*PageData
	Contents template.HTML
	Path     string
}

type CommitPageData struct {
	*PageData
	CommitMsg template.HTML
	CommitID  string
	Commit    *CommitData
	Diff      *DiffRender
	Parent    string
	ParentURL template.URL
	CommitURL template.URL
}

type RefPageData struct {
	*PageData
	Refs []*RefInfo
}

type WriteData struct {
	Template string
	Filename string
	Subdir   string
	Data     interface{}
}

func bail(err error) {
	if err != nil {
		panic(err)
	}
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

// converts contents of files in git tree to pretty formatted code
func parseText(filename string, text string, style *chroma.Style) (string, error) {
	formatter := formatterHtml.New(
		formatterHtml.WithLineNumbers(true),
		formatterHtml.LinkableLineNumbers(true, ""),
		formatterHtml.WithClasses(true),
	)
	lexer := lexers.Match(filename)
	if lexer == nil {
		lexer = lexers.Analyse(text)
	}
	if lexer == nil {
		lexer = lexers.Get("plaintext")
	}
	iterator, err := lexer.Tokenise(nil, text)
	if err != nil {
		return text, err
	}
	var buf bytes.Buffer
	err = formatter.Format(&buf, style, iterator)
	if err != nil {
		return text, err
	}
	return buf.String(), nil
}

// isText reports whether a significant prefix of s looks like correct UTF-8;
// that is, if it is likely that s is human-readable text.
func isText(s string) bool {
	const max = 1024 // at least utf8.UTFMax
	if len(s) > max {
		s = s[0:max]
	}
	for i, c := range s {
		if i+utf8.UTFMax > len(s) {
			// last char may be incomplete - ignore
			break
		}
		if c == 0xFFFD || c < ' ' && c != '\n' && c != '\t' && c != '\f' && c != '\r' {
			// decoding error or control character - not a text file
			return false
		}
	}
	return true
}

// isTextFile reports whether the file has a known extension indicating
// a text file, or if a significant chunk of the specified file looks like
// correct UTF-8; that is, if it is likely that the file contains human-
// readable text.
func isTextFile(text string) bool {
	num := math.Min(float64(len(text)), 1024)
	return isText(text[0:int(num)])
}

func toPretty(b int64) string {
	return humanize.Bytes(uint64(b))
}

func repoName(root string) string {
	_, file := filepath.Split(root)
	return file
}

func readmeFile(repo *Config) string {
	if repo.Readme == "" {
		return "readme.md"
	}

	return strings.ToLower(repo.Readme)
}

func walkTree(tree *git.Tree, branch string, curpath string, aggregate []*TreeItem) []*TreeItem {
	entries, err := tree.Entries()
	bail(err)

	for _, entry := range entries {
		fname := filepath.Join(curpath, entry.Name())
		typ := entry.Type()
		if typ == git.ObjectTree {
			re, _ := tree.Subtree(entry.Name())
			aggregate = walkTree(re, branch, fname, aggregate)
		}

		if entry.Type() == git.ObjectBlob {
			aggregate = append(aggregate, &TreeItem{
				Size:  toPretty(entry.Size()),
				Path:  fname,
				Entry: entry,
				URL:   template.URL(filepath.Join("/", "tree", branch, "item", fname)),
			})
		}
	}

	return aggregate
}

func (c *Config) writeHtml(writeData *WriteData) {
	ts, err := template.ParseFS(
		efs,
		writeData.Template,
		"html/header.partial.tmpl",
		"html/base.layout.tmpl",
	)
	bail(err)

	dir := filepath.Join(c.Outdir, c.RepoName, writeData.Subdir)
	err = os.MkdirAll(dir, os.ModePerm)
	bail(err)

	fp := filepath.Join(dir, writeData.Filename)
	c.Logger.Infof("writing (%s)", fp)

	w, err := os.OpenFile(fp, os.O_WRONLY|os.O_CREATE, 0755)
	bail(err)

	err = ts.Execute(w, writeData.Data)
	bail(err)
}

func (c *Config) writeRootSummary(data *PageData, readme template.HTML) {
	c.writeHtml(&WriteData{
		Filename: "index.html",
		Template: "html/summary.page.tmpl",
		Data: &SummaryPageData{
			PageData: data,
			Readme:   readme,
		},
	})
}

func (c *Config) writeTree(data *PageData, tree []*TreeItem) {
	c.writeHtml(&WriteData{
		Filename: "index.html",
		Subdir:   filepath.Join("tree", data.RevData.RevName),
		Template: "html/tree.page.tmpl",
		Data: &TreePageData{
			PageData: data,
			Tree:     tree,
		},
	})
}

func (c *Config) writeLog(data *PageData, logs []*CommitData) {
	c.writeHtml(&WriteData{
		Filename: "index.html",
		Subdir:   filepath.Join("logs", data.RevData.RevName),
		Template: "html/log.page.tmpl",
		Data: &LogPageData{
			PageData: data,
			Logs:     logs,
		},
	})
}

func (c *Config) writeRefs(data *PageData, refs []*RefInfo) {
	c.writeHtml(&WriteData{
		Filename: "refs.html",
		Template: "html/refs.page.tmpl",
		Data: &RefPageData{
			PageData: data,
			Refs:     refs,
		},
	})
}

func (c *Config) writeHTMLTreeFiles(pageData *PageData, tree []*TreeItem) string {
	readme := ""
	for _, file := range tree {
		b, err := file.Entry.Blob().Bytes()
		bail(err)
		str := string(b)

		file.IsTextFile = isTextFile(str)

		contents := "binary file, cannot display"
		if file.IsTextFile {
			file.NumLines = len(strings.Split(str, "\n"))
			contents, err = parseText(file.Entry.Name(), string(b), c.Theme)
			bail(err)
		}

		d := filepath.Dir(file.Path)

		nameLower := strings.ToLower(file.Entry.Name())
		summary := readmeFile(pageData.Repo)
		if nameLower == summary {
			readme = contents
		}

		c.writeHtml(&WriteData{
			Filename: fmt.Sprintf("%s.html", file.Entry.Name()),
			Template: "html/file.page.tmpl",
			Data: &FilePageData{
				PageData: pageData,
				Contents: template.HTML(contents),
				Path:     file.Path,
			},
			Subdir: filepath.Join("tree", pageData.RevData.RevName, "item", d),
		})
	}
	return readme
}

func (c *Config) writeLogDiffs(repo *git.Repository, pageData *PageData, logs []*CommitData) {
	for _, commit := range logs {
		commitID := commit.ID.String()

		if c.Cache[commitID] {
			c.Logger.Infof("(%s) commit file already generated, skipping", getShortID(commitID))
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
				URL:    c.getCommitURL(pt.ID.String()),
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
			finContent, err := parseText("commit.diff", content, c.Theme)
			bail(err)

			fl.Content = template.HTML(finContent)
			fls = append(fls, fl)
		}
		rnd.Files = fls

		commitData := &CommitPageData{
			PageData:  pageData,
			Commit:    commit,
			CommitID:  getShortID(commitID),
			Diff:      rnd,
			Parent:    getShortID(parentID),
			CommitURL: c.getCommitURL(commitID),
			ParentURL: c.getCommitURL(parentID),
		}

		c.writeHtml(&WriteData{
			Filename: fmt.Sprintf("%s.html", commitID),
			Template: "html/commit.page.tmpl",
			Subdir:   "commits",
			Data:     commitData,
		})
	}
}

func (c *Config) getCloneURL() template.URL {
	url := fmt.Sprintf("https://%s/%s.git", c.CloneURL, c.RepoName)
	return template.URL(url)
}

func (c *Config) getSummaryUrl() template.URL {
	url := fmt.Sprintf("/%s/index.html", c.RepoName)
	return template.URL(url)
}

func (c *Config) getRefsUrl() template.URL {
	url := fmt.Sprintf("/%s/refs.html", c.RepoName)
	return template.URL(url)
}

func (c *Config) getTreeUrl(revn string) template.URL {
	url := fmt.Sprintf("/%s/tree/%s/index.html", c.RepoName, revn)
	return template.URL(url)
}

func (c *Config) getLogsUrl(revn string) template.URL {
	url := fmt.Sprintf("/%s/logs/%s/index.html", c.RepoName, revn)
	return template.URL(url)
}

func (c *Config) getCommitURL(commitID string) template.URL {
	url := fmt.Sprintf("/%s/commits/%s.html", c.RepoName, commitID)
	return template.URL(url)
}

func (c *Config) getURLs() *SiteURLs {
	return &SiteURLs{
		RootURL:    c.HomeUrl,
		CloneURL:   c.CloneURL,
		RefsURL:    c.getRefsUrl(),
		SummaryURL: c.getSummaryUrl(),
	}
}

func getShortID(id string) string {
	return id[:7]
}

func (c *Config) writeRepo() *BranchOutput {
	repo, err := git.Open(c.RepoPath)
	bail(err)

	refs, err := repo.ShowRef(git.ShowRefOptions{Heads: true, Tags: true})
	bail(err)

	var first *RevData
	revs := []*RevData{}
	for _, revStr := range c.Revs {
		fullRevID, err := repo.RevParse(revStr)
		bail(err)

		revName := getShortID(fullRevID)
		// if it's a reference then label it as such
		for _, ref := range refs {
			if revStr == git.RefShortName(ref.Refspec) || revStr == ref.Refspec {
				revName = revStr
				break
			}
		}

		data := &RevData{
			ID:      fullRevID,
			RevName: revName,
			TreeURL: c.getTreeUrl(revName),
			LogURL:  c.getLogsUrl(revName),
		}
		if first == nil {
			first = data
		}
		revs = append(revs, data)
	}

	if first == nil {
		bail(fmt.Errorf("could find find a git reference that matches criteria"))
	}

	refInfoMap := map[string]*RefInfo{}
	mainOutput := &BranchOutput{}
	claimed := false
	for _, revData := range revs {
		refInfoMap[revData.RevName] = &RefInfo{
			ID: revData.ID,
			Refspec: revData.RevName,
			URL:     revData.TreeURL,
		}
	}

	// loop through ALL refs that don't have URLs
	// and add them to the map
	for _, ref := range refs {
		refspec := git.RefShortName(ref.Refspec)
		if refInfoMap[refspec] != nil {
			continue
		}

		refInfoMap[refspec] = &RefInfo{
			ID: ref.ID,
			Refspec: refspec,
		}
	}

	// gather lists of refs to display on refs.html page
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

	for _, revData := range revs {
	data := &PageData{
			Repo:     c,
			RevData:  revData,
			SiteURLs: c.getURLs(),
		}

		branchOutput := c.writeBranch(repo, data, refInfoList)
		if !claimed {
			mainOutput = branchOutput
			claimed = true
		}
	}

	// use the first revision in our list to generate
	// the root summary, logs, and tree the user can click
	revData := &RevData{
		TreeURL: c.getTreeUrl(first.RevName),
		LogURL:  c.getLogsUrl(first.RevName),
		RevName: first.RevName,
	}

	data := &PageData{
		RevData:  revData,
		Repo:     c,
		SiteURLs: c.getURLs(),
	}
	c.writeRefs(data, refInfoList)
	c.writeRootSummary(data, template.HTML(mainOutput.Readme))
	return mainOutput
}

func (c *Config) writeBranch(repo *git.Repository, pageData *PageData, refs []*RefInfo) *BranchOutput {
	fmt.Printf("compiling (%s) branch (%s)\n", c.RepoName, pageData.RevData.RevName)

	output := &BranchOutput{}
	pageSize := pageData.Repo.MaxCommits
	if pageSize == 0 {
		pageSize = 5000
	}

	commits, err := repo.CommitsByPage(pageData.RevData.ID, 0, pageSize)
	bail(err)

	logs := []*CommitData{}
	for i, commit := range commits {
		if i == 0 {
			output.LastCommit = commit
		}

		tags := []*RefInfo{}
		for _, ref := range refs {
			if commit.ID.String() == ref.ID {
				tags = append(tags, ref)
			}
		}

		logs = append(logs, &CommitData{
			URL:        c.getCommitURL(commit.ID.String()),
			ShortID:    getShortID(commit.ID.String()),
			SummaryStr: commit.Summary(),
			AuthorStr:  commit.Author.Name,
			WhenStr:    timediff.TimeDiff(commit.Author.When),
			Commit:     commit,
			Refs: tags,
		})
	}

	tree, err := repo.LsTree(pageData.RevData.ID)
	bail(err)

	entries := []*TreeItem{}
	treeEntries := walkTree(tree, pageData.RevData.RevName, "", entries)
	for _, entry := range treeEntries {
		entry.Path = strings.TrimPrefix(entry.Path, "/")

		var lastCommits []*git.Commit
		// `git rev-list` is pretty expensive here, so we have a flag to disable
		if pageData.Repo.HideTreeLastCommit {
			c.Logger.Info("skipping finding last commit for each file")
		} else {
			lastCommits, err = repo.RevList([]string{pageData.RevData.ID}, git.RevListOptions{
				Path:           entry.Path,
				CommandOptions: git.CommandOptions{Args: []string{"-1"}},
			})
			bail(err)

			var lc *git.Commit
			if len(lastCommits) > 0 {
				lc = lastCommits[0]
			}
			entry.CommitURL = c.getCommitURL(lc.ID.String())
			entry.Summary = lc.Summary()
			entry.When = timediff.TimeDiff(lc.Author.When)
		}
		entry.URL = template.URL(filepath.Join(
			"/",
			c.RepoName,
			"tree",
			pageData.RevData.RevName,
			"item",
			fmt.Sprintf("%s.html", entry.Path),
		))
	}

	c.Logger.Infof(
		"compilation complete (%s) branch (%s)",
		c.RepoName,
		pageData.RevData.RevName,
	)

	c.writeLog(pageData, logs)
	c.writeLogDiffs(repo, pageData, logs)
	c.writeTree(pageData, treeEntries)
	readme := c.writeHTMLTreeFiles(pageData, treeEntries)

	output.Readme = readme
	return output
}

func main() {
	var outdir = flag.String("out", "./public", "output directory")
	var rpath = flag.String("repo", ".", "path to git repo")
	var revsFlag = flag.String("revs", "HEAD", "list of revs to generate logs and tree (e.g. main,v1,c69f86f,HEAD")
	var themeFlag = flag.String("theme", "dracula", "theme to use for site")

	flag.Parse()

	out, err := filepath.Abs(*outdir)
	bail(err)
	repoPath, err := filepath.Abs(*rpath)
	bail(err)

	revs := strings.Split(*revsFlag, ",")
	if len(revs) == 1 && revs[0] == "" {
		revs = []string{}
	}

	if len(revs) == 0 {
		bail(fmt.Errorf("you must provide --revs"))
	}

	theme := styles.Get(*themeFlag)

	lg, err := zap.NewProduction()
	if err != nil {
		bail(err)
	}

	logger := lg.Sugar()

	config := &Config{
		Outdir:   out,
		RepoPath: repoPath,
		RepoName: repoName(repoPath),
		Cache:    make(map[string]bool),
		Revs:     revs,
		Theme:    theme,
		Logger:   logger,
	}
	config.Logger.Infof("%+v", config)

	config.writeRepo()
	url := filepath.Join("/", config.RepoName, "index.html")
	config.Logger.Info(url)
}
