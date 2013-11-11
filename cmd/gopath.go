package cmd

import (
	"go/build"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Unknwon/com"
	"github.com/codegangsta/cli"

	"github.com/gpmgo/gopm/doc"
	"github.com/gpmgo/gopm/log"
)

func getGopmPkgs(dirPath string) (pkgs map[string]*doc.Pkg, err error) {
	absPath, err := filepath.Abs(dirPath)
	if err != nil {
		log.Error("", "Fail to get absolute path of work directory")
		log.Fatal("", err.Error())
	}

	var builds map[string]string

	if com.IsFile(absPath + "/" + doc.GopmFileName) {
		gf := doc.NewGopmfile(absPath)

		if builds, err = gf.GetSection("deps"); err != nil {
			builds = nil
		}
	}

	pkg, err := build.ImportDir(dirPath, build.AllowBinary)
	if err != nil {
		return map[string]*doc.Pkg{}, err
	}

	pkgs = make(map[string]*doc.Pkg)
	for _, name := range pkg.Imports {
		if !doc.IsGoRepoPath(name) {
			if builds != nil {
				if dep, ok := builds[name]; ok {
					pkgs[name] = &doc.Pkg{ImportPath: dep}
					continue
				}
			}
			pkgs[name] = doc.NewDefaultPkg(name)
		}
	}
	return pkgs, nil
}

func pkgInCache(name string, cachePkgs map[string]*doc.Pkg) bool {
	//pkgs := strings.Split(name, "/")
	_, ok := cachePkgs[name]
	return ok
}

func autoLink(oldPath, newPath string) error {
	newPPath, _ := filepath.Split(newPath)
	os.MkdirAll(newPPath, os.ModePerm)
	return makeLink(oldPath, newPath)
}

func getChildPkgs(ctx *cli.Context, cpath string, ppkg *doc.Pkg, cachePkgs map[string]*doc.Pkg) error {
	pkgs, err := getGopmPkgs(cpath)
	if err != nil {
		return err
	}
	for name, pkg := range pkgs {
		if !pkgInCache(name, cachePkgs) {
			var newPath string
			if !build.IsLocalImport(name) {
				newPath = filepath.Join(installRepoPath, pkg.ImportPath)
				if pkgName != "" && strings.HasPrefix(pkg.ImportPath, pkgName) {
					newPath = filepath.Join(curPath, pkg.ImportPath[len(pkgName)+1:])
				} else {
					if !com.IsExist(newPath) {
						var t, ver string = doc.BRANCH, ""
						node := doc.NewNode(pkg.ImportPath, pkg.ImportPath, t, ver, true)
						nodes := []*doc.Node{node}
						downloadPackages(ctx, nodes)
						// should handler download failed
					}
				}
			} else {
				newPath, err = filepath.Abs(name)
				if err != nil {
					return err
				}
			}
			err = getChildPkgs(ctx, newPath, pkg, cachePkgs)
			if err != nil {
				return err
			}
		}
	}
	if ppkg != nil && !build.IsLocalImport(ppkg.ImportPath) {
		cachePkgs[ppkg.ImportPath] = ppkg
	}
	return nil
}

var pkgName string
var curPath string
var newCurPath string
var newGoPath string

func execCmd(gopath, curPath string, args ...string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	com.ColorLog("[INFO] change current dir from %v to %v\n", cwd, curPath)
	err = os.Chdir(filepath.Join(cwd, "vendor"))
	if err != nil {
		com.ColorLog("[ERRO] change current directory error %v\n", err)
		return err
	}
	err = os.Chdir(curPath)
	if err != nil {
		com.ColorLog("[ERRO] change current directory error %v\n", err)
		return err
	}
	defer os.Chdir(cwd)
	ccmd := exec.Command("cd", curPath)
	ccmd.Stdout = os.Stdout
	ccmd.Stderr = os.Stderr
	err = ccmd.Run()
	if err != nil {
		com.ColorLog("[ERRO] change current directory error %v\n", err)
		return err
	}

	oldGoPath := os.Getenv("GOPATH")
	com.ColorLog("[TRAC] set GOPATH from %v to %v\n", oldGoPath, gopath)

	err = os.Setenv("GOPATH", gopath)
	if err != nil {
		com.ColorLog("[ERRO] %v\n", err)
		return err
	}
	defer os.Setenv("GOPATH", oldGoPath)

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func genNewGoPath(ctx *cli.Context) {
	var err error
	curPath, err = os.Getwd()
	if err != nil {
		log.Error("", "Fail to get work directory")
		log.Fatal("", err.Error())
	}

	hd, err := com.HomeDir()
	if err != nil {
		log.Error("", "Fail to get current user")
		log.Fatal("", err.Error())
	}

	doc.HomeDir = strings.Replace(doc.RawHomeDir, "~", hd, -1)
	installRepoPath = doc.HomeDir + "/repos"

	if com.IsFile(curPath + "/" + doc.GopmFileName) {
		log.Trace("Loading gopmfile...")
		gf := doc.NewGopmfile(curPath)

		var err error
		pkgName, err = gf.GetValue("target", "path")
		if err == nil {
			log.Log("Target name: %s", pkgName)
		}
	}

	if len(pkgName) == 0 {
		_, pkgName = filepath.Split(curPath)
	}

	cachePkgs := make(map[string]*doc.Pkg)
	err = getChildPkgs(ctx, curPath, nil, cachePkgs)
	if err != nil {
		log.Error("", "Fail to get child pakcages")
		log.Fatal("", err.Error())
	}

	newGoPath = filepath.Join(curPath, "vendor")
	newGoPathSrc := filepath.Join(newGoPath, "src")
	os.RemoveAll(newGoPathSrc)
	os.MkdirAll(newGoPathSrc, os.ModePerm)

	for name, _ := range cachePkgs {
		oldPath := filepath.Join(installRepoPath, name)
		newPath := filepath.Join(newGoPathSrc, name)
		paths := strings.Split(name, "/")
		var isExistP bool
		var isCurChild bool
		for i := 0; i < len(paths)-1; i++ {
			pName := strings.Join(paths[:len(paths)-1-i], "/")
			if _, ok := cachePkgs[pName]; ok {
				isExistP = true
				break
			}
			if pkgName == pName {
				isCurChild = true
				break
			}
		}
		if isCurChild {
			continue
		}

		if !isExistP {
			log.Log("Linking %s", name)
			err = autoLink(oldPath, newPath)
			if err != nil {
				log.Error("", "Fail to make link")
				log.Fatal("", err.Error())
			}
		}
	}

	newCurPath = filepath.Join(newGoPathSrc, pkgName)
	log.Log("Linking %s", pkgName)
	err = autoLink(curPath, newCurPath)
	if err != nil {
		log.Error("", "Fail to make link")
		log.Fatal("", err.Error())
	}
}
