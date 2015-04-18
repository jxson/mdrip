package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
)

func dumpBucket(label string, bucket *ScriptBucket) {
	fmt.Printf("#\n# Script @%s from %s \n#\n", label, bucket.fileName)
	delimFmt := "#" + strings.Repeat("-", 70) + "#  %s %d\n"
	for i, block := range bucket.script {
		fmt.Printf(delimFmt, "Start", i+1)
		fmt.Printf("echo \"Block '%s' (%d/%d in %s) of %s\"\n####\n",
			block.labels[0], i+1, len(bucket.script), label, bucket.fileName)
		fmt.Print(block.codeText)
		fmt.Printf(delimFmt, "End", i+1)
		fmt.Println()
	}
}

func emitStraightScript(label string, scriptBuckets []*ScriptBucket) {
	for _, bucket := range scriptBuckets {
		dumpBucket(label, bucket)
	}
	fmt.Printf("echo \"All done.  No errors.\"\n")
}

// Emit the first script normally, but emit the remaining scripts so
// that they run in a subshell.
//
// This allows the aggregrate script to be structured as 1) a preamble
// initialization script that impacts the environment of the active
// shell, followed by 2) a script that executes as a subshell with
// error handling.
//
// This makes it possible to both modify the active shell (the shell
// running the aggregrate script) and allow exit on error *without*
// exiting the active shell.
//
// The first script must be able to complete without error, i.e. it
// should just define environment, because its not running as a
// subshell.  Anything that checks the environment can go in the
// subshell.
func emitPreambledScript(label string, scriptBuckets []*ScriptBucket) {
	dumpBucket(label, scriptBuckets[0])
	delim := "HANDLED_SCRIPT"
	fmt.Printf(" bash -e <<'%s'\n", delim)
	fmt.Printf("function superTrouble() { echo \"Unable to continue!\"; exit 1; }\n")
	fmt.Printf("trap superTrouble INT TERM EXIT\n")
	for i := 1; i < len(scriptBuckets); i++ {
		dumpBucket(label, scriptBuckets[i])
	}
	fmt.Printf("echo \"All done.  No errors.\"\n")
	fmt.Printf("%s\n", delim)
}

func usage() {
	fmt.Fprintf(os.Stderr, "\nUsage:  %s {label} {fileName}...\n", os.Args[0])
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr,
		`
Reads markdown files, extracts code blocks with a given @label, and
either runs them in a subshell or emits them to stdout.

If the markdown file contains

  Blah blah blah.
  <!-- @goHome @foo -->
  '''
  cd $HOME
  '''
  Blah blah blah.
  <!-- @echoApple @apple -->
  '''
  echo "an apple a day keeps the doctor away"
  '''
  Blah blah blah.
  <!-- @echoCloseStar @foo @baz -->
  '''
  echo "Proxima Centauri"
  '''
  Blah blah blah.

then the command '{this} foo {fileName}' emits: 

  cd $HOME
  echo "Proxima Centauri"

Pipe output to 'source /dev/stdin' to run it directly.

Use --subshell to run the blocks in a subshell leaving your current
shell env vars and pwd unchanged.  The code blocks can, however, do
anything to your computer that you can.
`)
}

func main() {
	flag.Usage = usage
	preambled := flag.Bool("preambled", false,
		"Place all scripts but first script into a subshell program.")
	subshell := flag.Bool("subshell", false,
		"Run extracted blocks in subshell (leaves your env vars and pwd unchanged).")
	swallow := flag.Bool("swallow", false,
		"Swallow errors from subshell (non-zero exit only on problems in driver code).")
	flag.Parse()
	if *swallow && !*subshell {
		fmt.Fprintf(os.Stderr, "Makes no sense to specify --swallow but not --subshell.\n")
		usage()
		os.Exit(1)
	}
	if flag.NArg() < 2 {
		usage()
		os.Exit(1)
	}
	label := flag.Arg(0)
	scriptBuckets := make([]*ScriptBucket, flag.NArg()-1)

	for i := 1; i < flag.NArg(); i++ {
		fileName := flag.Arg(i)
		contents, err := ioutil.ReadFile(fileName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to read %q\n", fileName)
			usage()
			os.Exit(2)
		}
		m := Parse(string(contents))
		script, ok := m[label]
		if !ok {
			fmt.Fprintf(os.Stderr, "No block labelled %q in file %q.\n", label, fileName)
			os.Exit(3)
		}
		scriptBuckets[i-1] = &ScriptBucket{fileName, script}
	}

	if len(scriptBuckets) < 1 {
		return
	}

	if !*subshell {
		if *preambled {
			emitPreambledScript(label, scriptBuckets)
		} else {
			emitStraightScript(label, scriptBuckets)
		}
		return
	}

	result := RunInSubShell(scriptBuckets)
	if result.err != nil {
		Complain(result, label)
		if !*swallow {
			log.Fatal(result.err)
		}
	}
}
