package indexer

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/blevesearch/bleve"
	"github.com/dgplug/dexer/lib/conf"
	"github.com/dgplug/dexer/lib/logger"
	"github.com/radovskyb/watcher"
)

// FileIndexer is a data structure to hold the content of the file
type FileIndexer struct {
	FileName    string
	FileContent string
}

type FileIndexerArray struct {
	IndexerArray    []FileIndexer
	FileIndexLogger *logger.Logger
}

func Search(indexFilename string, searchWord string) *bleve.SearchResult {
	index, _ := bleve.Open(indexFilename)
	defer index.Close()
	query := bleve.NewQueryStringQuery(searchWord)
	request := bleve.NewSearchRequest(query)
	result, _ := index.Search(request)
	return result
}

func fileIndexing(fileIndexer FileIndexerArray, c conf.Configuration) {
	err := DeleteExistingIndex(c.IndexFilename)
	fileIndexer.FileIndexLogger.Must(err, "Successfully deleted previous index")
	mapping := bleve.NewIndexMapping()
	index, err := bleve.New(c.IndexFilename, mapping)
	fileIndexer.FileIndexLogger.Must(err, "Successfully ran bleve for indexing")
	for _, fileIndex := range fileIndexer.IndexerArray {
		index.Index(fileIndex.FileName, fileIndex.FileContent)
	}
	defer index.Close()
}

func fileNameContentMap(c conf.Configuration) FileIndexerArray {
	var root = c.RootDirectory
	var files []string
	fileIndexer := FileIndexerArray{
		FileIndexLogger: c.LogMan,
	}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	fileIndexer.FileIndexLogger.Must(err, "Successfully traversed "+root)
	for _, filename := range files {
		content, err := GetContent(filename)
		fileIndexer.FileIndexLogger.Must(err, "Successfully obtained content from "+filename)
		filesIndex := NewFileIndexer(filename, content)
		fileIndexer.IndexerArray = append(fileIndexer.IndexerArray, filesIndex)
	}
	return fileIndexer
}

// NewFileIndexer is a function to create a new File Indexer
func NewFileIndexer(fname, fcontent string) FileIndexer {
	temp := FileIndexer{
		FileName:    fname,
		FileContent: fcontent,
	}

	return temp
}

// NewIndex is a function to create new indexes
func NewIndex(c conf.Configuration) {

	fileIndexer := fileNameContentMap(c)
	fileIndexing(fileIndexer, c)

	c.LogMan.Must(nil, "Refreshing the index")
	w := watcher.New()
	w.FilterOps(watcher.Rename, watcher.Move, watcher.Create, watcher.Remove, watcher.Write)

	go func() {
		for {
			select {
			case event := <-w.Event:
				c.LogMan.Must(nil, event.Name())
				fileIndexer := fileNameContentMap(c)
				fileIndexing(fileIndexer, c)
			case err := <-w.Error:
				c.LogMan.Must(err, "")
			case <-w.Closed:
				return
			}
		}
	}()

	err := w.AddRecursive(c.RootDirectory)
	c.LogMan.Must(err, "Successfully added "+c.RootDirectory+" to the watcher")

	go func() {
		w.Wait()
	}()

	err = w.Start(time.Millisecond * 100)
	c.LogMan.Must(err, "Successfully started the watcher")
}

// GetContent is a function for retrieving data from file
func GetContent(name string) (string, error) {
	data, err := ioutil.ReadFile(name)
	return string(data), err
}

// DeleteExistingIndex checks if the index exist if it does, then flushes it off
func DeleteExistingIndex(name string) error {
	_, err := os.Stat(name)
	if !os.IsNotExist(err) {
		if err := os.RemoveAll(name); err != nil {
			return fmt.Errorf("Can't Delete file: %v", err)
		}
	}
	return nil
}
