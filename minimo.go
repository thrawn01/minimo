package main

import (
	"github.com/jessevdk/go-flags"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

var version = "0.1.0"

// Holds the Stats of a file
type FileStats struct {
	name     string
	fileInfo os.FileInfo
}

type Config struct {
	IncludeFiles []string `yaml:"include-files"`
	ExcludeFiles []string `yaml:"exclude-files"`
}

// Command line argument definition
var opts struct {
	Verbose bool   `short:"v" long:"verbose" description:"Show verbose debug information"`
	Conf    string `short:"c" long:"conf" description:"path to the config file" env:"MINIMO_CONF" required:"yes"`
	Args    struct {
		Path string `positional-arg-name:"PATH"`
	} `positional-args:"yes" required:"yes"`
}

// Walks the path provided and return a slice of fileStat structs
func walkpath(rootPath string) map[string]FileStats {
	var results = make(map[string]FileStats)

	// Walk the rootPath
	filepath.Walk(rootPath, func(path string, fileInfo os.FileInfo, err error) error {
		if err != nil {
			log.Fatal(err)
		}
		// Skip the dot file
		if path == "." {
			return nil
		}
		// Create a new FileStats struct and add it to the list
		results[path] = FileStats{name: path, fileInfo: fileInfo}
		return nil
	})

	return results
}

// Compare directory structure before and after filesystem modification.
func main() {
	// Parse command line arguments
	_, err := flags.Parse(&opts)
	if err != nil {
		os.Exit(-1)
	}

	// Read the config file
	rawConfFile, err := ioutil.ReadFile(opts.Conf)
	if err != nil {
		log.Fatal(err)
	}
	// Unmarshal the yaml file into our config struct
	conf := Config{}
	err = yaml.Unmarshal(rawConfFile, &conf)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("#%v\n", conf)
	os.Exit(-1)

	// Get a listing of the file that exists before modification
	before := walkpath(opts.Args.Path)

	// Install the requested packages
	err = exec.Command("/usr/bin/touch", "newFile.txt").Run()
	if err != nil {
		log.Fatal(err)
	}

	// List the file system contents again
	after := walkpath(opts.Args.Path)

	// Look for files that existed before, but now deleted
	for key, value := range before {
		_, exists := after[key]
		if !exists {
			log.Printf("DELETED '%s' - bytes: %d mtime: (%s)\n", key, value.fileInfo.Size(), value.fileInfo.ModTime().Format(time.UnixDate))
		}
	}

	for key, value := range after {
		// Look for files that existed before, but now deleted
		_, exists := before[key]
		if !exists {
			log.Printf("NEW '%s' - bytes: %d mtime: (%s)\n", key, value.fileInfo.Size(), value.fileInfo.ModTime().Format(time.UnixDate))
		}
	}
}
