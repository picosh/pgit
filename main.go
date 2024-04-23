package main

import (
	"bytes"
	"embed"
	_ "embed"
	"flag"
	"fmt"
	"html/template"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/alecthomas/chroma"
	formatterHtml "github.com/alecthomas/chroma/formatters/html"
	"github.com/alecthomas/chroma/lexers"
	"github.com/alecthomas/chroma/styles"
	"github.com/dustin/go-humanize"
	git "github.com/gogs/git-module"
)

//go:embed html/*.tmpl static/*
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
	HomeURL  template.URL
	CloneURL template.URL

	// computed
	// cache for skipping commits, trees, etc.
	Cache map[string]bool
	// mutex for Cache
	Mutex sync.RWMutex
	// pretty name for the repo
	RepoName string
	// logger
	Logger *slog.Logger
	// chroma style
	Theme     *chroma.Style
	Formatter *formatterHtml.Formatter
}

type RevInfo interface {
	ID() string
	Name() string
}

// revision data
type RevData struct {
	id   string
	name string
}

func (r *RevData) ID() string {
	return r.id
}

func (r *RevData) Name() string {
	return r.name
}

func (r *RevData) TreeURL() template.URL {
	return getTreeURL(r)
}

func (r *RevData) LogURL() template.URL {
	return getLogsURL(r)
}

type TagData struct {
	Name string
	URL  template.URL
}

type CommitData struct {
	SummaryStr string
	URL        template.URL
	WhenStr    string
	AuthorStr  string
	ShortID    string
	ParentID   string
	Refs       []*RefInfo
	*git.Commit
}

type TreeItem struct {
	IsTextFile bool
	IsDir      bool
	Size       string
	NumLines   int
	Name       string
	Icon       string
	Path       string
	URL        template.URL
	CommitID   string
	CommitURL  template.URL
	Summary    string
	When       string
	Author     *git.Signature
	Entry      *git.TreeEntry
	Crumbs     []*Breadcrumb
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
	ID      string
	Refspec string
	URL     template.URL
}

type BranchOutput struct {
	Readme     string
	LastCommit *git.Commit
}

type SiteURLs struct {
	HomeURL    template.URL
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
	Tree *TreeRoot
}

type LogPageData struct {
	*PageData
	NumCommits int
	Logs       []*CommitData
}

type FilePageData struct {
	*PageData
	Contents template.HTML
	Item     *TreeItem
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
func (c *Config) parseText(filename string, text string) (string, error) {
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
	err = c.Formatter.Format(&buf, c.Theme, iterator)
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

func (c *Config) writeHtml(writeData *WriteData) {
	ts, err := template.ParseFS(
		efs,
		writeData.Template,
		"html/header.partial.tmpl",
		"html/footer.partial.tmpl",
		"html/base.layout.tmpl",
	)
	bail(err)

	dir := filepath.Join(c.Outdir, writeData.Subdir)
	err = os.MkdirAll(dir, os.ModePerm)
	bail(err)

	fp := filepath.Join(dir, writeData.Filename)
	c.Logger.Info("writing", "filepath", fp)

	w, err := os.OpenFile(fp, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	bail(err)

	err = ts.Execute(w, writeData.Data)
	bail(err)
}

func (c *Config) copyStatic(dir string) error {
	entries, err := efs.ReadDir(dir)
	bail(err)

	for _, e := range entries {
		infp := filepath.Join(dir, e.Name())
		if e.IsDir() {
			continue
		}

		w, err := efs.ReadFile(infp)
		bail(err)
		fp := filepath.Join(c.Outdir, e.Name())
		c.Logger.Info("writing", "filepath", fp)
		os.WriteFile(fp, w, 0644)
	}

	return nil
}

func (c *Config) writeRootSummary(data *PageData, readme template.HTML) {
	c.Logger.Info("writing root html", "repoPath", c.RepoPath)
	c.writeHtml(&WriteData{
		Filename: "index.html",
		Template: "html/summary.page.tmpl",
		Data: &SummaryPageData{
			PageData: data,
			Readme:   readme,
		},
	})
}

func (c *Config) writeTree(data *PageData, tree *TreeRoot) {
	c.Logger.Info("writing tree", "treePath", tree.Path)
	c.writeHtml(&WriteData{
		Filename: "index.html",
		Subdir:   tree.Path,
		Template: "html/tree.page.tmpl",
		Data: &TreePageData{
			PageData: data,
			Tree:     tree,
		},
	})
}

func (c *Config) writeLog(data *PageData, logs []*CommitData) {
	c.Logger.Info("writing log file", "revision", data.RevData.Name())
	c.writeHtml(&WriteData{
		Filename: "index.html",
		Subdir:   getLogBaseDir(data.RevData),
		Template: "html/log.page.tmpl",
		Data: &LogPageData{
			PageData:   data,
			NumCommits: len(logs),
			Logs:       logs,
		},
	})
}

func (c *Config) writeRefs(data *PageData, refs []*RefInfo) {
	c.Logger.Info("writing refs", "repoPath", c.RepoPath)
	c.writeHtml(&WriteData{
		Filename: "refs.html",
		Template: "html/refs.page.tmpl",
		Data: &RefPageData{
			PageData: data,
			Refs:     refs,
		},
	})
}

func (c *Config) writeHTMLTreeFile(pageData *PageData, treeItem *TreeItem) string {
	readme := ""
	b, err := treeItem.Entry.Blob().Bytes()
	bail(err)
	str := string(b)

	treeItem.IsTextFile = isTextFile(str)

	contents := "binary file, cannot display"
	if treeItem.IsTextFile {
		treeItem.NumLines = len(strings.Split(str, "\n"))
		contents, err = c.parseText(treeItem.Entry.Name(), string(b))
		bail(err)
	}

	d := filepath.Dir(treeItem.Path)

	nameLower := strings.ToLower(treeItem.Entry.Name())
	summary := readmeFile(pageData.Repo)
	if nameLower == summary {
		readme = contents
	}

	c.writeHtml(&WriteData{
		Filename: fmt.Sprintf("%s.html", treeItem.Entry.Name()),
		Template: "html/file.page.tmpl",
		Data: &FilePageData{
			PageData: pageData,
			Contents: template.HTML(contents),
			Item:     treeItem,
		},
		Subdir: getFileURL(pageData.RevData, d),
	})
	return readme
}

func (c *Config) writeLogDiff(repo *git.Repository, pageData *PageData, commit *CommitData) {
	commitID := commit.ID.String()

	c.Mutex.RLock()
	hasCommit := c.Cache[commitID]
	c.Mutex.RUnlock()

	if hasCommit {
		c.Logger.Info("commit file already generated, skipping", "commitID", getShortID(commitID))
		return
	} else {
		c.Mutex.Lock()
		c.Cache[commitID] = true
		c.Mutex.Unlock()
	}

	diff, err := repo.Diff(
		commitID,
		0,
		0,
		0,
		git.DiffOptions{},
	)
	bail(err)

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
		finContent, err := c.parseText("commit.diff", content)
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
		Parent:    getShortID(commit.ParentID),
		CommitURL: getCommitURL(commitID),
		ParentURL: getCommitURL(commit.ParentID),
	}

	c.writeHtml(&WriteData{
		Filename: fmt.Sprintf("%s.html", commitID),
		Template: "html/commit.page.tmpl",
		Subdir:   "commits",
		Data:     commitData,
	})
}

func getSummaryURL() template.URL {
	url := "/index.html"
	return template.URL(url)
}

func getRefsURL() template.URL {
	url := "/refs.html"
	return template.URL(url)
}

// controls the url for trees and logs
// /logs/getRevIDForURL()/index.html
// /tree/getRevIDForURL()/item/file.x.html
func getRevIDForURL(info RevInfo) string {
	return info.Name()
}

func getTreeBaseDir(info RevInfo) string {
	subdir := getRevIDForURL(info)
	return filepath.Join("/", "tree", subdir)
}

func getLogBaseDir(info RevInfo) string {
	subdir := getRevIDForURL(info)
	return filepath.Join("/", "logs", subdir)
}

func getFileBaseDir(info RevInfo) string {
	return filepath.Join(getTreeBaseDir(info), "item")
}

func getFileURL(info RevInfo, fname string) string {
	return filepath.Join(getFileBaseDir(info), fname)
}

func getTreeURL(info RevInfo) template.URL {
	dir := getTreeBaseDir(info)
	url := filepath.Join(dir, "index.html")
	return template.URL(url)
}

func getLogsURL(info RevInfo) template.URL {
	dir := getLogBaseDir(info)
	url := filepath.Join(dir, "index.html")
	return template.URL(url)
}

func getCommitURL(commitID string) template.URL {
	url := fmt.Sprintf("/commits/%s.html", commitID)
	return template.URL(url)
}

func (c *Config) getURLs() *SiteURLs {
	return &SiteURLs{
		HomeURL:    c.HomeURL,
		CloneURL:   c.CloneURL,
		RefsURL:    getRefsURL(),
		SummaryURL: getSummaryURL(),
	}
}

func getShortID(id string) string {
	return id[:7]
}

func (c *Config) writeRepo() *BranchOutput {
	c.Logger.Info("writing repo", "repoPath", c.RepoPath)
	repo, err := git.Open(c.RepoPath)
	bail(err)

	refs, err := repo.ShowRef(git.ShowRefOptions{Heads: true, Tags: true})
	bail(err)

	var first *RevData
	revs := []*RevData{}
	for _, revStr := range c.Revs {
		fullRevID, err := repo.RevParse(revStr)
		bail(err)

		revID := getShortID(fullRevID)
		revName := revID
		// if it's a reference then label it as such
		for _, ref := range refs {
			if revStr == git.RefShortName(ref.Refspec) || revStr == ref.Refspec {
				revName = revStr
				break
			}
		}

		data := &RevData{
			id:   fullRevID,
			name: revName,
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
		refInfoMap[revData.Name()] = &RefInfo{
			ID:      revData.ID(),
			Refspec: revData.Name(),
			URL:     revData.TreeURL(),
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
			ID:      ref.ID,
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
		c.Logger.Info("writing revision", "revision", revData.Name())
		data := &PageData{
			Repo:     c,
			RevData:  revData,
			SiteURLs: c.getURLs(),
		}

		if claimed {
			go func() {
				c.writeRevision(repo, data, refInfoList)
			}()
		} else {
			branchOutput := c.writeRevision(repo, data, refInfoList)
			mainOutput = branchOutput
			claimed = true
		}
	}

	// use the first revision in our list to generate
	// the root summary, logs, and tree the user can click
	revData := &RevData{
		id:   first.ID(),
		name: first.Name(),
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

type TreeRoot struct {
	Path   string
	Items  []*TreeItem
	Crumbs []*Breadcrumb
}

type TreeWalker struct {
	treeItem           chan *TreeItem
	tree               chan *TreeRoot
	HideTreeLastCommit bool
	PageData           *PageData
	Repo               *git.Repository
}

type Breadcrumb struct {
	Text   string
	URL    template.URL
	IsLast bool
}

func (tw *TreeWalker) calcBreadcrumbs(curpath string) []*Breadcrumb {
	if curpath == "" {
		return []*Breadcrumb{}
	}
	parts := strings.Split(curpath, string(os.PathSeparator))
	rootURL := template.URL(
		filepath.Join(
			getTreeBaseDir(tw.PageData.RevData),
			"index.html",
		),
	)

	crumbs := make([]*Breadcrumb, len(parts)+1)
	crumbs[0] = &Breadcrumb{
		URL:  rootURL,
		Text: tw.PageData.Repo.RepoName,
	}

	cur := ""
	for idx, d := range parts {
		crumbs[idx+1] = &Breadcrumb{
			Text: d,
			URL:  template.URL(filepath.Join(getFileBaseDir(tw.PageData.RevData), cur, d, "index.html")),
		}
		if idx == len(parts)-1 {
			crumbs[idx+1].IsLast = true
		}
		cur = filepath.Join(cur, d)
	}

	return crumbs
}

func FilenameToDevIcon(filename string) string {
	ext := filepath.Ext(filename)
	extMappr := map[string]string{
		".html": "html5",
		".go":   "go",
		".py":   "python",
		".css":  "css3",
		".js":   "javascript",
		".md":   "markdown",
		".ts":   "typescript",
		".tsx":  "react",
		".jsx":  "react",
	}

	nameMappr := map[string]string{
		"Makefile":   "cmake",
		"Dockerfile": "docker",
	}

	icon := extMappr[ext]
	if icon == "" {
		icon = nameMappr[filename]
	}

	return fmt.Sprintf("devicon-%s-original", icon)
}

func (tw *TreeWalker) NewTreeItem(entry *git.TreeEntry, curpath string, crumbs []*Breadcrumb) *TreeItem {
	typ := entry.Type()
	fname := filepath.Join(curpath, entry.Name())
	item := &TreeItem{
		Size:   toPretty(entry.Size()),
		Name:   entry.Name(),
		Path:   fname,
		Entry:  entry,
		URL:    template.URL(getFileURL(tw.PageData.RevData, fname)),
		Crumbs: crumbs,
	}

	// `git rev-list` is pretty expensive here, so we have a flag to disable
	if tw.HideTreeLastCommit {
		// c.Logger.Info("skipping the process of finding the last commit for each file")
	} else {
		id := tw.PageData.RevData.ID()
		lastCommits, err := tw.Repo.RevList([]string{id}, git.RevListOptions{
			Path:           item.Path,
			CommandOptions: git.CommandOptions{Args: []string{"-1"}},
		})
		bail(err)

		var lc *git.Commit
		if len(lastCommits) > 0 {
			lc = lastCommits[0]
		}
		item.CommitURL = getCommitURL(lc.ID.String())
		item.CommitID = getShortID(lc.ID.String())
		item.Summary = lc.Summary()
		item.When = lc.Author.When.Format("02 Jan 06")
		item.Author = lc.Author
	}

	fpath := getFileURL(
		tw.PageData.RevData,
		fmt.Sprintf("%s.html", fname),
	)
	if typ == git.ObjectTree {
		item.IsDir = true
		fpath = filepath.Join(
			getFileBaseDir(tw.PageData.RevData),
			curpath,
			entry.Name(),
			"index.html",
		)
	} else if typ == git.ObjectBlob {
		item.Icon = FilenameToDevIcon(item.Name)
	}
	item.URL = template.URL(fpath)

	return item
}

func (tw *TreeWalker) walk(tree *git.Tree, curpath string) {
	entries, err := tree.Entries()
	bail(err)

	crumbs := tw.calcBreadcrumbs(curpath)
	treeEntries := []*TreeItem{}
	for _, entry := range entries {
		typ := entry.Type()
		item := tw.NewTreeItem(entry, curpath, crumbs)

		if typ == git.ObjectTree {
			item.IsDir = true
			re, _ := tree.Subtree(entry.Name())
			tw.walk(re, item.Path)
			treeEntries = append(treeEntries, item)
			tw.treeItem <- item
		} else if typ == git.ObjectBlob {
			treeEntries = append(treeEntries, item)
			tw.treeItem <- item
		}
	}

	sort.Slice(treeEntries, func(i, j int) bool {
		nameI := treeEntries[i].Name
		nameJ := treeEntries[j].Name
		if treeEntries[i].IsDir && treeEntries[j].IsDir {
			return nameI < nameJ
		}

		if treeEntries[i].IsDir && !treeEntries[j].IsDir {
			return true
		}

		if !treeEntries[i].IsDir && treeEntries[j].IsDir {
			return false
		}

		return nameI < nameJ
	})

	fpath := filepath.Join(
		getFileBaseDir(tw.PageData.RevData),
		curpath,
	)
	// root gets a special spot outside of `item` subdir
	if curpath == "" {
		fpath = getTreeBaseDir(tw.PageData.RevData)
	}

	tw.tree <- &TreeRoot{
		Path:   fpath,
		Items:  treeEntries,
		Crumbs: crumbs,
	}

	if curpath == "" {
		close(tw.tree)
		close(tw.treeItem)
	}
}

func (c *Config) writeRevision(repo *git.Repository, pageData *PageData, refs []*RefInfo) *BranchOutput {
	c.Logger.Info(
		"compiling revision",
		"repoName", c.RepoName,
		"revision", pageData.RevData.Name(),
	)

	output := &BranchOutput{}

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()

		pageSize := pageData.Repo.MaxCommits
		if pageSize == 0 {
			pageSize = 5000
		}
		commits, err := repo.CommitsByPage(pageData.RevData.ID(), 0, pageSize)
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

			parentSha, _ := commit.ParentID(0)
			parentID := ""
			if parentSha == nil {
				parentID = commit.ID.String()
			} else {
				parentID = parentSha.String()
			}
			logs = append(logs, &CommitData{
				ParentID:   parentID,
				URL:        getCommitURL(commit.ID.String()),
				ShortID:    getShortID(commit.ID.String()),
				SummaryStr: commit.Summary(),
				AuthorStr:  commit.Author.Name,
				WhenStr:    commit.Author.When.Format("02 Jan 06"),
				Commit:     commit,
				Refs:       tags,
			})
		}

		c.writeLog(pageData, logs)

		for _, cm := range logs {
			wg.Add(1)
			go func(commit *CommitData) {
				defer wg.Done()
				c.writeLogDiff(repo, pageData, commit)
			}(cm)
		}
	}()

	tree, err := repo.LsTree(pageData.RevData.ID())
	bail(err)

	readme := ""
	entries := make(chan *TreeItem)
	subtrees := make(chan *TreeRoot)
	tw := &TreeWalker{
		PageData: pageData,
		Repo:     repo,
		treeItem: entries,
		tree:     subtrees,
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		tw.walk(tree, "")
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for e := range entries {
			wg.Add(1)
			go func(entry *TreeItem) {
				defer wg.Done()
				if entry.IsDir {
					return
				}

				readmeStr := c.writeHTMLTreeFile(pageData, entry)
				if readmeStr != "" {
					readme = readmeStr
				}
			}(e)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for t := range subtrees {
			wg.Add(1)
			go func(tree *TreeRoot) {
				defer wg.Done()
				c.writeTree(pageData, tree)
			}(t)
		}
	}()

	wg.Wait()

	c.Logger.Info(
		"compilation complete branch",
		"repoName", c.RepoName,
		"revision", pageData.RevData.Name(),
	)

	output.Readme = readme
	return output
}

func main() {
	var outdir = flag.String("out", "./public", "output directory")
	var rpath = flag.String("repo", ".", "path to git repo")
	var revsFlag = flag.String("revs", "HEAD", "list of revs to generate logs and tree (e.g. main,v1,c69f86f,HEAD")
	var themeFlag = flag.String("theme", "dracula", "theme to use for site")
	var labelFlag = flag.String("label", "", "pretty name for the subdir where we create the repo, default is last folder in --repo")
	var cloneFlag = flag.String("clone-url", "", "git clone URL")
	var homeFlag = flag.String("home-url", "", "URL for breadcumbs to get to list of repositories")
	var descFlag = flag.String("desc", "", "description for repo")
	var maxCommitsFlag = flag.Int("max-commits", 0, "maximum number of commits to generate")
	var hideTreeLastCommitFlag = flag.Bool("hide-tree-last-commit", false, "dont calculate last commit for each file in the tree")

	flag.Parse()

	out, err := filepath.Abs(*outdir)
	bail(err)
	repoPath, err := filepath.Abs(*rpath)
	bail(err)

	theme := styles.Get(*themeFlag)

	logger := slog.Default()

	label := repoName(repoPath)
	if *labelFlag != "" {
		label = *labelFlag
	}

	revs := strings.Split(*revsFlag, ",")
	if len(revs) == 1 && revs[0] == "" {
		revs = []string{}
	}

	formatter := formatterHtml.New(
		formatterHtml.WithLineNumbers(true),
		formatterHtml.LinkableLineNumbers(true, ""),
		formatterHtml.WithClasses(true),
	)

	config := &Config{
		Outdir:             out,
		RepoPath:           repoPath,
		RepoName:           label,
		Cache:              make(map[string]bool),
		Revs:               revs,
		Theme:              theme,
		Logger:             logger,
		CloneURL:           template.URL(*cloneFlag),
		HomeURL:            template.URL(*homeFlag),
		Desc:               *descFlag,
		MaxCommits:         *maxCommitsFlag,
		HideTreeLastCommit: *hideTreeLastCommitFlag,
		Formatter:          formatter,
	}
	config.Logger.Info("config", "config", config)

	if len(revs) == 0 {
		bail(fmt.Errorf("you must provide --revs"))
	}

	config.writeRepo()
	config.copyStatic("static")

	fp := filepath.Join(out, "syntax.css")
	w, err := os.OpenFile(fp, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		bail(err)
	}
	err = formatter.WriteCSS(w, theme)
	if err != nil {
		bail(err)
	}

	url := filepath.Join("/", "index.html")
	config.Logger.Info("root url", "url", url)
}
