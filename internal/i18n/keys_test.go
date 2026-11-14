package i18n

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// keyHelpers are the calls whose arguments at these positions are text keys, beside i18n.T and
// i18n.Has. A new helper taking a key belongs here, so that a typo in its keys is caught.
var keyHelpers = map[string][]int{
	"button":       {0},
	"small":        {0},
	"subCaption":   {0},
	"menuItem":     {0},
	"valueRow":     {0},
	"aboutRow":     {0},
	"optionRow":    {0, 1},
	"selectRow":    {0, 1},
	"keyedSection": {0, 1},
	"addKeyed":     {0, 1},
	"menuCaption":  {1},
	"earSlider":    {2},
	"side":         {0},
	"translated":   {0},
}

// dynamicPrefixes are the families of keys the code builds at run time, each with where it is
// built: nothing names their members literally. A family no key belongs to fails the test.
var dynamicPrefixes = map[string]string{
	"setting.": "settingLabel and fitModel, by setting name",
	"word.":    "wordLabel, by the setting's value word",
	"mode.":    "wordLabel, by the mode's word",
	"chip.":    "chipLabel, by the mode's word",
	"help.":    "settingHelp, by setting name",
	"group.":   "cardTitle, by the card's name",
	"diag.":    "newDiagTab, by section and field name",
	"game.":    "gamePhaseText, by dongle.GameModeLabel",
}

// sourceRoots are walked from this package's directory.
var sourceRoots = []string{"../../internal", "../../cmd"}

// isTableFile leaves out the tables themselves, whose keys would all read as used.
func isTableFile(path string) bool {
	return filepath.Base(filepath.Dir(path)) == "i18n" && filepath.Base(path) != "i18n.go"
}

// literal is the string value of an expression, when it is a string literal.
func literal(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	return value, err == nil
}

// calledName is the name a call goes by, and whether it is i18n.T or i18n.Has, either from
// outside the package or from inside it.
func calledName(call *ast.CallExpr) (string, bool) {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name, fun.Name == "T" || fun.Name == "Has"
	case *ast.SelectorExpr:
		pkg, ok := fun.X.(*ast.Ident)
		isText := ok && pkg.Name == "i18n" && (fun.Sel.Name == "T" || fun.Sel.Name == "Has")
		return fun.Sel.Name, isText
	}
	return "", false
}

// usage is what the source says about keys.
type usage struct {
	// asKeys are literals in a key's position, with where they are.
	asKeys map[string]token.Position
	// mentioned are all string literals, since a key can reach T through a table or a variable.
	mentioned map[string]token.Position
}

func readUsage(t *testing.T) usage {
	t.Helper()
	found := usage{asKeys: map[string]token.Position{}, mentioned: map[string]token.Position{}}
	files := token.NewFileSet()
	for _, root := range sourceRoots {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || isTableFile(path) {
				return nil
			}
			source, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			file, err := parser.ParseFile(files, path, source, 0)
			if err != nil {
				return err
			}
			ast.Inspect(file, func(node ast.Node) bool {
				switch item := node.(type) {
				case *ast.BasicLit:
					if value, ok := literal(item); ok {
						found.mentioned[value] = files.Position(item.Pos())
					}
				case *ast.CallExpr:
					name, isText := calledName(item)
					positions := keyHelpers[name]
					if isText {
						positions = []int{0}
					}
					for _, index := range positions {
						if index < len(item.Args) {
							if value, ok := literal(item.Args[index]); ok && value != "" {
								found.asKeys[value] = files.Position(item.Args[index].Pos())
							}
						}
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return found
}

// family is the part of a key before its first dot.
func family(key string) string {
	head, _, _ := strings.Cut(key, ".")
	return head
}

// keyShaped is what a key looks like; anything else in the source is some other string.
var keyShaped = regexp.MustCompile(`^[a-z]+\.[A-Za-z][A-Za-z0-9 ]*$`)

// notKeys look like keys and are not.
var notKeys = map[string]bool{"tray.png": true}

// TestEveryKeyInTheCodeHasAText keeps a typo in a key from showing on the page as the key itself.
func TestEveryKeyInTheCodeHasAText(t *testing.T) {
	found := readUsage(t)
	if len(found.asKeys) < 100 {
		t.Fatalf("only %d keys found, so the walk is broken", len(found.asKeys))
	}
	for key, where := range found.asKeys {
		if !Has(key) {
			t.Errorf("%s: no text for %q", where, key)
		}
	}
	families := map[string]bool{}
	for key := range tables[English] {
		families[family(key)] = true
	}
	// A literal outside a known call is still a key if it looks like one of a known family.
	for value, where := range found.mentioned {
		if keyShaped.MatchString(value) && families[family(value)] && !Has(value) && !notKeys[value] {
			t.Errorf("%s: %q reads like a key, and has no text", where, value)
		}
	}
}

// TestEveryTextIsUsed keeps the tables free of texts nothing can show.
func TestEveryTextIsUsed(t *testing.T) {
	found := readUsage(t)
	dynamic := func(key string) bool {
		for prefix := range dynamicPrefixes {
			if strings.HasPrefix(key, prefix) {
				return true
			}
		}
		return false
	}
	for _, key := range slices.Sorted(maps.Keys(tables[English])) {
		if _, used := found.mentioned[key]; !used && !dynamic(key) {
			t.Errorf("no code uses %q", key)
		}
	}
	for prefix, where := range dynamicPrefixes {
		member := false
		for key := range tables[English] {
			member = member || strings.HasPrefix(key, prefix)
		}
		if !member {
			t.Errorf("no key starts with %q, said to come from %s", prefix, where)
		}
	}
}

// The walk has to see the kinds of code it relies on, or the tests above pass by seeing nothing.
func TestTheWalkSeesKeysInEveryPosition(t *testing.T) {
	found := readUsage(t)
	for _, key := range []string{
		"language.system", // T inside this package
		"tray.quit",       // i18n.T in another package
		"capture.title",   // w.subCaption
		"capture.help",    // w.small
		"audio.apply",     // w.button
		"audio.applyHint", // the second key of w.optionRow
	} {
		if _, ok := found.asKeys[key]; !ok {
			t.Errorf("did not see %q", key)
		}
	}
}
