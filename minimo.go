package main

import (
	"github.com/jessevdk/go-flags"
	//"gopkg.in/yaml.v2"
	"fmt"
	//"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

var Version = "0.0.1"

// Holds the Stats of a file
type FileStats struct {
	name     string
	fileInfo os.FileInfo
}

type Config struct {
	Verbose bool `short:"v" long:"verbose" description:"Show verbose debug information"`
	Args    struct {
		Conf string `positional-arg-name:"CONF" description:"path to the config file" env:"MINIMO_CONF" default:"config.yaml"`
	} `positional-args:"yes"`

	Platform     string   `long:"platform" description:"Platform to create the image for (Currently only supports debian/ubuntu)" default:"ubuntu"`
	IncludeFiles []string `long:"include-file" description:"Specify a file to include in the resulting image"`
	ExcludeFiles []string `long:"exclude-file" description:"Specify a file to exclude from the resulting image"`
	IncludePkgs  []string `long:"include-pkg" description:"Specify additional package to install in the resulting image"`
	ExcludePkgs  []string `long:"exclude-pkg" description:"Specify package remove from the resulting image, even if it has as a package dependancy"`
	Debian       struct {
		ScratchDir string `long:"scratch-dir" description:"Scratch directory where the bootstrap root is created" default:"/var/minimo"`
		Suite      string `long:"distro" description:"<suite> argument passed to 'debootstrap'" default:"utopic"`
		Mirror     string `long:"mirror" description:"<mirror> argument passed to 'debootstrap'" default:"http://archive.ubuntu.com/ubuntu/"`
		Variant    string `long:"variant" description:"--variant argument passed to 'debootstrap'" default:"buildd"`
		Arch       string `long:"arch" description:"--arch argument passed to 'debootstrap'" default:"amd64"`
		Bin        string `long:"bootstrap-bin" description:"Path to 'debootstrap'" default:"/usr/sbin/debootstrap"`
	} `group:"Debian Bootstrap Options"`
}

/*func loadConfig(fileName string) Config {
	// Read the entire config file
	rawConfFile, err := ioutil.ReadFile(fileName)
	if err != nil {
		log.Fatal(err)
	}
	// Unmarshal the yaml file into our config struct
	conf := Config{}
	err = yaml.Unmarshal(rawConfFile, &conf)
	if err != nil {
		log.Fatal(err)
	}
	return conf
}*/

// TODO: The scratch dir should probably be a tempdir that gets removed each run
func createDebianRoot(conf Config) string {
	debConf := conf.Debian

	// Create our scratch dir if doesn't exist
	rootPath := fmt.Sprintf("%s/%s", debConf.ScratchDir, debConf.Suite)
	if _, err := os.Stat(rootPath); os.IsNotExist(err) {
		err := os.Mkdir(rootPath, 775)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		// Always start with a clean root
		err := os.RemoveAll(rootPath)
		if err != nil {
			log.Fatal(err)
		}
	}

	// debootstrap --variant=buildd --arch amd64 utopic /var/minimo/utopic http://archive.ubuntu.com/ubuntu/
	cmd := exec.Command(debConf.Bin, "--variant", debConf.Variant, "--arch", debConf.Arch, debConf.Suite, rootPath, debConf.Mirror)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		log.Fatal(err)
	}
	return rootPath
}

func installDebianPkgs(conf Config) {

	// Prepare the chroot
	/*
		mount --bind /dev /var/minimo/utopic/dev
		mount --bind /dev/pts /var/minimo/utopic/dev/pts
		mount --bind /proc /var/minimo/utopic/proc/
		mount --bind /sys /var/minimo/utopic/sys
	*/
	for _, pkg := range conf.IncludePkgs {
		log.Printf("Installing %s", pkg)
		//
		// apt-get install --admindir /var/minimo/utopic/var/lib/dpkg --root /var/minimo/utopic -i
	}
}

func removeDebianPkgs(conf Config) {
	for _, pkg := range conf.ExcludePkgs {
		log.Printf("Removing %s", pkg)
	}
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
	conf := Config{}
	// Parse command line arguments
	_, err := flags.Parse(&conf)
	if err != nil {
		if err.(*flags.Error).Type == flags.ErrHelp {
			os.Exit(-1)
		}
		log.Fatal(err)
	}

	// Load our config file
	//conf := loadConfig(opts.Conf)
	//log.Printf("#%v\n", conf)

	// Create a rootfs
	rootPath := createDebianRoot(conf)

	// Get a listing of the base debian system prior to requested package installation
	before := walkpath(rootPath)

	// Install the requested packages
	installDebianPkgs(conf)
	// Remove any requested package ignoring any depedencies
	removeDebianPkgs(conf)

	/*newFile := fmt.Sprintf("%s/newFile.txt", rootPath)
	err = exec.Command("/usr/bin/touch", newFile).Run()
	if err != nil {
		log.Fatal(err)
	}*/

	// Get a listing off all the files after package installation
	after := walkpath(rootPath)

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
