package main

import (
	"fmt"
	//"github.com/fatih/structs"
	"github.com/jessevdk/go-flags"
	"gopkg.in/fatih/set.v0"
	"gopkg.in/lxc/go-lxc.v1"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var Version = "0.0.1"

type Config struct {
	Verbose           bool `short:"v" long:"verbose" description:"Show verbose debug information"`
	KeepTempContainer bool `short:"k" long:"keep-temp" description:"Do not delete the temporary container used to create the image"`
	Args              struct {
		Conf string `positional-arg-name:"CONF" description:"path to the config file" env:"MINIMO_CONF" default:"config.yaml"`
	} `positional-args:"yes"`

	Platform         string   `long:"platform" description:"platform to create the image for (Currently only supports debian/ubuntu)" default:"ubuntu"`
	IncludeFiles     []string `long:"include-file" description:"specify a file to include in the resulting image"`
	ExcludeFiles     []string `long:"exclude-file" description:"specify a file to exclude from the resulting image"`
	IncludePkgs      []string `long:"include-pkg" description:"specify additional package to install in the resulting image"`
	ExcludePkgs      []string `long:"exclude-pkg" description:"specify package remove from the resulting image, even if it has as a package dependancy"`
	UseTempContainer string   `long:"use-temp-container" description:"Do not create a new temp container, instead use this pre-existing LXC temp container to install packages (useful when debuging issues)"`
	Apt              struct {
		BuildDir string `long:"build-dir" description:"scratch directory where the bootstrap root is created" default:"/var/minimo"`
		Distro   string `long:"distro" description:"name of a dpkg based distro" default:"ubuntu"`
		Mirror   string `long:"mirror" description:"APT mirror endpoint" default:"http://archive.ubuntu.com/ubuntu/"`
		Release  string `long:"release" description:"release name" default:"utopic"`
		Arch     string `long:"arch" description:"which arch to install packages for" default:"amd64"`
	} `group:"dpkg"`
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

// Build a dependency list based on the list of packages requested to install
func buildDependencyList(container *lxc.Container, pkgs []string) set.Interface {
	regex, err := regexp.Compile(`(Depends|PreDepends): (\w+)`)
	if err != nil {
		log.Fatal("Unable to compile regexp for buildDependencyList()")
	}

	results := set.New()
	// For each package in the IncludePkgs list
	for _, pkg := range pkgs {
		// Fetch it's dependency list
		output, err := executeInContainer(container, "apt-cache", "depends", pkg)
		if err != nil {
			log.Fatalf("Error while fetching dependancy for '%s' - '%s'", pkg, err)
		}
		depends := make([]string, 5)
		matches := regex.FindAllStringSubmatch(output, -1)
		for match, pkgName := range matches {
			log.Printf("Match: %s Package: %s", match, pkgName)
			//depends = append(depends, pkgName)
		}
		log.Println(depends)
		// recursively decend the dependancy tree
		//results.Merge(buildDependencyList(depends))
	}
	return results
}

// Execute executes the given command in a temporary container.
func executeInContainer(container *lxc.Container, args ...string) (string, error) {
	cargs := []string{"lxc-execute", "-n", container.Name(), "-P", container.ConfigPath(), "--"}
	cargs = append(cargs, args...)

	cmd := exec.Command(cargs[0], cargs[1:]...)
	log.Printf("Executing: %s\n", strings.Join(cmd.Args, " "))
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func randName() string {
	rand := uint32(time.Now().UnixNano() + int64(os.Getpid()))
	rand = (rand * 1664525) + 1013904223 // constants from Numerical Recipes
	return strconv.Itoa(int(1e9 + (rand % 1e9)))[1:]
}

// TODO: The scratch dir should probably be a tempdir that gets removed each run
func createContainerHandle(conf Config) (string, *lxc.Container) {
	// TODO: Generate a temp container name every time
	containerName := randName()

	// Overide the temp container name if the user asks
	if len(conf.UseTempContainer) > 0 {
		containerName = conf.UseTempContainer
	}

	imagePath := fmt.Sprintf("%s/%s/rootfs", conf.Apt.BuildDir, containerName)

	// Create new image
	container, err := lxc.NewContainer(containerName, conf.Apt.BuildDir)
	if err != nil {
		log.Fatal(err)
	}
	// TODO: Load the default config file?
	container.LoadConfigFile(lxc.DefaultConfigPath())
	// Always be verbose
	container.SetVerbosity(lxc.Verbose)
	return imagePath, container
}

func createAptImage(conf Config, container *lxc.Container) {
	if container.Defined() {
		log.Printf("Temp Container '%s' already exists", container.Name())
		return
	}
	log.Println("Creating Temporary Image...")

	// TODO: Validate the 'distro' option has a valid LXC template
	if err := container.Create(conf.Apt.Distro, "-a", conf.Apt.Arch, "-r", conf.Apt.Release, "--mirror", conf.Apt.Mirror); err != nil {
		log.Fatalf("Error during temp image create '%s'", err)
		log.Fatal(err)
	}
	log.Println("Temporary Image Complete...")
}

func installAptPkgs(conf Config, container *lxc.Container) {
	pkgs := strings.Join(conf.IncludePkgs, " ")
	log.Printf("Installing %s", pkgs)

	cargs := []string{"apt-get", "install", "-y"}
	cargs = append(cargs, conf.IncludePkgs...)
	output, err := executeInContainer(container, cargs...)
	fmt.Println(output)
	if err != nil {
		log.Fatalf("Error while installing '%s' - '%s'", pkgs, err)
	}
}

func removeAptPkgs(conf Config, container *lxc.Container) {
	for _, pkg := range conf.ExcludePkgs {
		log.Printf("Removing %s", pkg)
	}
}

// Walks the path provided and return a slice of fileStat structs
func walkpath(rootPath string, ignoreRegexes []*regexp.Regexp) map[string]os.FileInfo {
	var results = make(map[string]os.FileInfo)

	// Walk the rootPath
	filepath.Walk(rootPath, func(path string, fileInfo os.FileInfo, err error) error {
		if err != nil {
			log.Fatal(err)
		}
		// Remove the relative path
		path = path[len(rootPath):]
		// Skip any files/directories that match our ignore list
		for _, regex := range ignoreRegexes {
			if regex.MatchString(path) {
				return nil
			}
		}
		results[path] = fileInfo
		return nil
	})

	return results
}

func buildModifiedFiles(preInstalledFiles map[string]os.FileInfo, postInstalledFiles map[string]os.FileInfo) []os.FileInfo {
	var modifiedFiles = make([]os.FileInfo, 500)
	// Look for files that where changed or deleted during the package installation
	for key, preFile := range preInstalledFiles {
		postFile, exists := postInstalledFiles[key]
		switch {
		case !exists:
			log.Printf("DELETED '%s' - bytes: %d mtime: (%s)\n", key, preFile.Size(), preFile.ModTime().Format(time.UnixDate))
		// Did any of the existing files get updated or modified?
		case postFile.Size() != preFile.Size():
			{
				log.Printf("CHANGED '%s' - Size '%d' to '%d'\n", key, preFile.Size(), postFile.Size())
				modifiedFiles = append(modifiedFiles, postFile)
			}
		case postFile.Mode() != preFile.Mode():
			{
				log.Printf("CHANGED '%s' - Mode '%d' to '%d'\n", key, preFile.Mode(), postFile.Mode())
				modifiedFiles = append(modifiedFiles, postFile)
			}
		case postFile.IsDir() != preFile.IsDir():
			{
				log.Printf("CHANGED '%s' - IsDir '%t' to '%t'\n", key, preFile.IsDir(), postFile.IsDir())
				modifiedFiles = append(modifiedFiles, postFile)
			}
		case postFile.ModTime() != preFile.ModTime():
			{
				log.Printf("CHANGED '%s' - ModTime '%s' to '%s'\n", key,
					preFile.ModTime().Format(time.UnixDate),
					postFile.ModTime().Format(time.UnixDate))
				modifiedFiles = append(modifiedFiles, postFile)
			}
		}
	}

	for key, postFile := range postInstalledFiles {
		// Look for files that existed before, but now deleted
		_, exists := preInstalledFiles[key]
		if !exists {
			log.Printf("NEW '%s' - bytes: %d mtime: (%s)\n", key, postFile.Size(), postFile.ModTime().Format(time.UnixDate))
			modifiedFiles = append(modifiedFiles, postFile)
		}
	}
	return modifiedFiles
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

	// TODO: run lxc-checkconfig to ensure lxc is working properly

	// Load our config file
	//conf := loadConfig(opts.Conf)
	//log.Printf("#%v\n", conf)

	ignoreRegexes := []*regexp.Regexp{
		regexp.MustCompile(`^$`),
		regexp.MustCompile(`^\.$`),
		regexp.MustCompile(`^/dev.*$`),
	}

	// Create an LXC container to handle image creation
	imagePath, container := createContainerHandle(conf)
	// Clean up the container when we exit
	defer func() {
		if err := container.Destroy(); err != nil {
			log.Fatalln(err.Error())
		}
		lxc.PutContainer(container)
		if !conf.KeepTempContainer {
			os.RemoveAll(imagePath)
		}
	}()

	log.Printf("Temp Image Path: '%s'", imagePath)

	// Create image
	createAptImage(conf, container)

	// Get a listing of the base debian system prior to requested package installation
	preInstalledFiles := walkpath(imagePath, ignoreRegexes)
	//log.Printf("preInstalledFiles: %s", preInstalledFiles)

	// Install the requested packages
	installAptPkgs(conf, container)
	// Remove any requested package ignoring any depedencies
	removeAptPkgs(conf, container)

	// Get a listing off all the files after package installation
	postInstalledFiles := walkpath(imagePath, ignoreRegexes)
	log.Printf("postInstalledFiles: %s", postInstalledFiles)

	// Build a list of files that where modified or created during the install process
	modifiedFiles := buildModifiedFiles(preInstalledFiles, postInstalledFiles)

	log.Printf("modifiedFile len: %d", len(modifiedFiles))
	log.Printf("modifiedFiles: %s", modifiedFiles)
	for _, fileInfo := range modifiedFiles {
		log.Printf("Modified: %s", fileInfo.Name())
	}

	// Build a list of packages based on the dependencies of the install package
	depends := buildDependencyList(container, conf.IncludePkgs)
	fmt.Println(depends)
	// Build a list of files that our install package depends on
	//dependantFiles = buildFileList(depends)

	// Merge the depedant files with the list of files that where modified during our installation
	//sparedFiles = merge(modifiedFiles, dependantFiles)
}
