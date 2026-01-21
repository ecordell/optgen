// Package main implements optgen, a code generator for functional options in Go.
//
// optgen generates functional option patterns for Go structs, including:
//   - With* functions for setting field values
//   - DebugMap methods for safe debug output
//   - Special handling for slices, maps, and sensitive fields
//
// Usage:
//
//	optgen [flags] <package-path> <struct-name> [<struct-name>...]
//
// Flags:
//
//	-output <path>
//	    Location where generated options will be written (required)
//	-package <name>
//	    Name of package to use in output file (optional, inferred from output directory)
//	-sensitive-field-name-matches <substring>
//	    Comma-separated list of field name substrings considered sensitive (default: "secure")
//
// Example:
//
//	//go:generate go run github.com/ecordell/optgen -output=config_options.go . Config
//
// Struct Tag Format:
//
// Fields must be annotated with the `debugmap` struct tag:
//   - "visible" - Show actual field value in DebugMap
//   - "visible-format" - Show formatted value (expands collections)
//   - "sensitive" - Show "(sensitive)" placeholder
//   - "hidden" - Omit from DebugMap entirely
//
// Example struct:
//
//	type Config struct {
//	    Name     string `debugmap:"visible"`
//	    Password string `debugmap:"sensitive"`
//	    Data     []byte `debugmap:"hidden"`
//	}
package main

import (
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	_ "github.com/creasty/defaults"
	"github.com/dave/jennifer/jen"
	"github.com/fatih/structtag"
)

type WriterProvider func() io.Writer

// TODO: struct tags to know what to generate
// TODO: recursive generation, i.e. WithMetadata(WithName())
// TODO: optional flattening of recursive generation, i.e. WithMetadataName()
// TODO: configurable field prefix
// TODO: exported / unexported generation

var DefaultSensitiveNames = "secure"

func main() {
	fs := flag.NewFlagSet("optgen", flag.ContinueOnError)
	outputPathFlag := fs.String(
		"output",
		"",
		"Location where generated options will be written",
	)
	pkgNameFlag := fs.String(
		"package",
		"",
		"Name of package to use in output file",
	)
	sensitiveFieldNamesFlag := fs.String(
		"sensitive-field-name-matches",
		DefaultSensitiveNames,
		"Substring matches of field names that should be considered sensitive",
	)

	if err := fs.Parse(os.Args[1:]); err != nil {
		log.Fatal(err.Error())
	}

	if len(fs.Args()) < 2 {
		// TODO: usage
		log.Fatal("must specify a package directory and a struct to provide options for")
	}

	pkgName := fs.Arg(0)
	structNames := fs.Args()[1:]
	structFilter := make(map[string]struct{}, len(structNames))
	for _, structName := range structNames {
		structFilter[structName] = struct{}{}
	}

	var writer WriterProvider
	if outputPathFlag != nil {
		writer = func() io.Writer {
			w, err := os.OpenFile(*outputPathFlag, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o600)
			if err != nil {
				log.Fatalf("couldn't open %s for writing", *outputPathFlag)
			}
			return w
		}
	}

<<<<<<< ours
	packagePath, packageName := func() (string, string) {
		cfg := &packages.Config{
			Mode: packages.NeedTypes | packages.NeedTypesInfo,
=======
	// Determine package name from output directory or flag
	packageName := func() string {
		if pkgNameFlag != nil && *pkgNameFlag != "" {
			return *pkgNameFlag
>>>>>>> theirs
		}
		// Parse a Go file in the output directory to get package name
		outputDir := filepath.Dir(*outputPathFlag)
		fset := token.NewFileSet()
		pkgs, err := parser.ParseDir(fset, outputDir, nil, parser.PackageClauseOnly)
		if err != nil || len(pkgs) == 0 {
			return "main" // fallback
		}
		for name := range pkgs {
			return name
		}
		return "main"
	}()

	sensitiveNameMatches := make([]string, 0)
	if sensitiveFieldNamesFlag != nil {
		sensitiveNameMatches = strings.Split(*sensitiveFieldNamesFlag, ",")
	}

	err := func() error {
<<<<<<< ours
		cfg := &packages.Config{
			Mode: packages.NeedFiles | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedSyntax,
		}
		pkgs, err := packages.Load(cfg, pkgName)
=======
		fset := token.NewFileSet()
		pkgs, err := parser.ParseDir(fset, pkgName, nil, parser.ParseComments)
>>>>>>> theirs
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse: %v\n", err)
			os.Exit(1)
		}

		count := 0
		for _, pkg := range pkgs {
			for _, f := range pkg.Files {
				structs := findStructDefsAST(f, structFilter)
				if len(structs) == 0 {
					continue
				}
				fmt.Printf("Generating options for %s.%s...\n", packageName, strings.Join(structNames, ", "))
				err = generateForFileAST(f, structs, packageName, f.Name.Name, *outputPathFlag, sensitiveNameMatches, writer)
				if err != nil {
					return err
				}
				count++
			}
		}
		if count == 0 {
			return errors.New("no structs found")
		}
		return nil
	}()
	if err != nil {
		log.Fatal(err)
	}
}

// findStructDefsAST finds struct type definitions in an AST file that match the given names.
// It returns a slice of *ast.TypeSpec for each matching struct type.
func findStructDefsAST(file *ast.File, names map[string]struct{}) []*ast.TypeSpec {
	found := make([]*ast.TypeSpec, 0)
	ast.Inspect(file, func(node ast.Node) bool {
		var ts *ast.TypeSpec
		var ok bool

		if ts, ok = node.(*ast.TypeSpec); !ok {
			return true
		}

		if ts.Name == nil {
			return true
		}

		if _, ok := names[ts.Name.Name]; !ok {
			return false
		}

		// Check if it's a struct type
		if _, isStruct := ts.Type.(*ast.StructType); isStruct {
			found = append(found, ts)
		}

		return false
	})

	return found
}

type Config struct {
	ReceiverId     string
	OptTypeName    string
	TargetTypeName string
	StructRef      []jen.Code
	StructName     string
	PkgPath        string
}

const (
	DebugMapFieldTag = "debugmap"
)

// ImportResolver maps package names to their full import paths
type ImportResolver struct {
	pkgToPath map[string]string
}

// NewImportResolver creates an ImportResolver from a file's imports.
// The resolver maps package names to their full import paths, handling both
// standard imports and aliased imports.
func NewImportResolver(file *ast.File) *ImportResolver {
	resolver := &ImportResolver{pkgToPath: make(map[string]string)}
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)

		// Determine package name
		var pkgName string
		if imp.Name != nil {
			pkgName = imp.Name.Name // Aliased import
		} else {
			// Extract last component: "database/sql" → "sql"
			pkgName = filepath.Base(path)
		}

		resolver.pkgToPath[pkgName] = path
	}
	return resolver
}

// Resolve returns the full import path for a package name.
// For example, "sql" might resolve to "database/sql".
func (r *ImportResolver) Resolve(pkgName string) string {
	if path, ok := r.pkgToPath[pkgName]; ok {
		return path
	}
	// Fallback for standard library single-component imports
	return pkgName
}

// parseStructTag parses a struct field tag and returns the value for the given key.
// Returns an error if the tag is missing or cannot be parsed.
func parseStructTag(field *ast.Field, tagKey string) (string, error) {
	if field.Tag == nil {
		return "", fmt.Errorf("missing tag")
	}
	// field.Tag.Value is like `debugmap:"visible"` (includes backticks)
	tagStr := strings.Trim(field.Tag.Value, "`")
	tags, err := structtag.Parse(tagStr)
	if err != nil {
		return "", err
	}
	tag, err := tags.Get(tagKey)
	if err != nil {
		return "", err
	}
	return tag.Value(), nil
}

// generateForFileAST generates functional options code for the given struct types.
// It creates option types, constructor functions, and utility methods for each struct.
func generateForFileAST(file *ast.File, typeSpecs []*ast.TypeSpec, pkgName, fileName, outpath string, sensitiveNameMatches []string, writer WriterProvider) error {
	outdir, err := filepath.Abs(filepath.Dir(outpath))
	if err != nil {
		return err
	}

	// Create import resolver for cross-package types
	resolver := NewImportResolver(file)

	buf := jen.NewFilePathName(outpath, pkgName)
	buf.PackageComment("Code generated by github.com/ecordell/optgen. DO NOT EDIT.")

	for _, ts := range typeSpecs {
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return errors.New("type is not a struct")
		}

		structName := ts.Name.Name
		config := Config{
			ReceiverId:     strings.ToLower(string(structName[0])),
			OptTypeName:    fmt.Sprintf("%sOption", structName),
			TargetTypeName: strings.Title(structName),
			StructRef:      []jen.Code{jen.Id(structName)},
			StructName:     structName,
			PkgPath:        "", // Not needed for AST-based generation
		}

		// generate the Option type
		writeOptionTypeAST(buf, config)

		// generate NewXWithOptions
		writeNewXWithOptionsAST(buf, config)

		// generate NewXWithOptionsAndDefaults
		writeNewXWithOptionsAndDefaultsAST(buf, config)

		// generate ToOption
		writeToOptionAST(buf, st, config)

		// generate DebugMap
		writeDebugMapAST(buf, st, config, sensitiveNameMatches)

		// generate WithOptions
		writeXWithOptionsAST(buf, config)
		writeWithOptionsAST(buf, config)

		// generate all With* functions
		writeAllWithOptFuncsAST(buf, st, outdir, config, resolver)
	}

	w := writer()
	if w == nil {
		optFile := strings.Replace(fileName, ".go", "_opts.go", 1)
		w, err = os.OpenFile(optFile, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o600)
		if err != nil {
			return err
		}
	}

	return buf.Render(w)
}

func writeOptionTypeAST(buf *jen.File, c Config) {
	buf.Type().Id(c.OptTypeName).Func().Params(jen.Id(c.ReceiverId).Op("*").Add(c.StructRef...))
}

func writeNewXWithOptionsAST(buf *jen.File, c Config) {
	newFuncName := fmt.Sprintf("New%sWithOptions", c.TargetTypeName)
	buf.Comment(fmt.Sprintf("%s creates a new %s with the passed in options set", newFuncName, c.StructName))
	buf.Func().Id(newFuncName).Params(
		jen.Id("opts").Op("...").Id(c.OptTypeName),
	).Op("*").Add(c.StructRef...).BlockFunc(func(grp *jen.Group) {
		grp.Id(c.ReceiverId).Op(":=").Op("&").Add(c.StructRef...).Block()
		applyOptions(c.ReceiverId)(grp)
	})
}

func writeNewXWithOptionsAndDefaultsAST(buf *jen.File, c Config) {
	newFuncName := fmt.Sprintf("New%sWithOptionsAndDefaults", c.TargetTypeName)
	buf.Comment(fmt.Sprintf("%s creates a new %s with the passed in options set starting from the defaults", newFuncName, c.StructName))
	buf.Func().Id(newFuncName).Params(
		jen.Id("opts").Op("...").Id(c.OptTypeName),
	).Op("*").Add(c.StructRef...).BlockFunc(func(grp *jen.Group) {
		grp.Id(c.ReceiverId).Op(":=").Op("&").Add(c.StructRef...).Block()
		grp.Qual("github.com/creasty/defaults", "MustSet").Call(jen.Id(c.ReceiverId))
		applyOptions(c.ReceiverId)(grp)
	})
}

func writeToOptionAST(buf *jen.File, st *ast.StructType, c Config) {
	newFuncName := fmt.Sprintf("ToOption")

	buf.Comment(fmt.Sprintf("%s returns a new %s that sets the values from the passed in %s", newFuncName, c.OptTypeName, c.StructName))
	buf.Func().Params(jen.Id(c.ReceiverId).Op("*").Id(c.StructName)).Id(newFuncName).Params().Id(c.OptTypeName).BlockFunc(func(grp *jen.Group) {
		grp.Return(jen.Func().Params(jen.Id("to").Op("*").Id(c.StructName)).BlockFunc(func(retGrp *jen.Group) {
			for _, field := range st.Fields.List {
				for _, name := range field.Names {
					if name.IsExported() {
						retGrp.Id("to").Op(".").Id(name.Name).Op("=").Id(c.ReceiverId).Op(".").Id(name.Name)
					}
				}
			}
		}))
	})
}

func writeDebugMapAST(buf *jen.File, st *ast.StructType, c Config, sensitiveNameMatches []string) {
	newFuncName := fmt.Sprintf("DebugMap")

	buf.Comment(fmt.Sprintf("%s returns a map form of %s for debugging", newFuncName, c.TargetTypeName))
	buf.Func().Params(jen.Id(c.ReceiverId).Op("*").Id(c.StructName)).Id(newFuncName).Params().Id("map[string]any").BlockFunc(func(grp *jen.Group) {
		mapId := "debugMap"
		grp.Id(mapId).Op(":=").Map(jen.String()).Any().Values()

		for _, field := range st.Fields.List {
			// Skip anonymous fields
			if field.Names == nil {
				continue
			}

			for _, name := range field.Names {
				// Skip unexported fields
				if !name.IsExported() {
					continue
				}

				fieldName := name.Name

				// Parse the debugmap tag
				tagValue, err := parseStructTag(field, DebugMapFieldTag)
				if err != nil {
					fmt.Printf("missing debugmap tag on field %s in type %s\n", fieldName, c.TargetTypeName)
					os.Exit(1)
				}

				switch tagValue {
				case "visible":
					// Check that sensitive field names are not marked as visible
					for _, sensitiveName := range sensitiveNameMatches {
						if strings.Contains(strings.ToLower(fieldName), sensitiveName) {
							fmt.Printf("field %s in type %s must be marked as 'sensitive'\n", fieldName, c.TargetTypeName)
							os.Exit(1)
						}
					}

					grp.Id(mapId).Index(jen.Lit(fieldName)).Op("=").Qual("github.com/ecordell/optgen/helpers", "DebugValue").Call(jen.Id(c.ReceiverId).Dot(fieldName), jen.Lit(false))

				case "visible-format":
					// Check that sensitive field names are not marked as visible-format
					for _, sensitiveName := range sensitiveNameMatches {
						if strings.Contains(strings.ToLower(fieldName), sensitiveName) {
							fmt.Printf("field %s in type %s must be marked as 'sensitive'\n", fieldName, c.TargetTypeName)
							os.Exit(1)
						}
					}

					grp.Id(mapId).Index(jen.Lit(fieldName)).Op("=").Qual("github.com/ecordell/optgen/helpers", "DebugValue").Call(jen.Id(c.ReceiverId).Dot(fieldName), jen.Lit(true))

				case "hidden":
					// Skip this field entirely
					continue

				case "sensitive":
					grp.Id(mapId).Index(jen.Lit(fieldName)).Op("=").Qual("github.com/ecordell/optgen/helpers", "SensitiveDebugValue").Call(jen.Id(c.ReceiverId).Dot(fieldName))

				default:
					fmt.Printf("unknown value '%s' for debugmap tag on field %s in type %s\n", tagValue, fieldName, c.TargetTypeName)
					os.Exit(1)
				}
			}
		}

		grp.Return(jen.Id(mapId))
	})
}

func writeXWithOptionsAST(buf *jen.File, c Config) {
	withFuncName := fmt.Sprintf("%sWithOptions", c.TargetTypeName)
	buf.Comment(fmt.Sprintf("%s configures an existing %s with the passed in options set", withFuncName, c.StructName))
	buf.Func().Id(withFuncName).Params(
		jen.Id(c.ReceiverId).Op("*").Add(c.StructRef...), jen.Id("opts").Op("...").Id(c.OptTypeName),
	).Op("*").Add(c.StructRef...).BlockFunc(applyOptions(c.ReceiverId))
}

func writeWithOptionsAST(buf *jen.File, c Config) {
	withFuncName := "WithOptions"
	buf.Comment(fmt.Sprintf("%s configures the receiver %s with the passed in options set", withFuncName, c.StructName))
	buf.Func().Params(jen.Id(c.ReceiverId).Op("*").Id(c.StructName)).Id(withFuncName).
		Params(jen.Id("opts").Op("...").Id(c.OptTypeName)).Op("*").Add(c.StructRef...).
		BlockFunc(applyOptions(c.ReceiverId))
}

func writeAllWithOptFuncsAST(buf *jen.File, st *ast.StructType, outdir string, c Config, resolver *ImportResolver) {
	for _, field := range st.Fields.List {
		if field.Names == nil {
			// Anonymous field, skip
			continue
		}

		for _, name := range field.Names {
			if name.IsExported() {
				fieldName := name.Name

				// Try to convert AST type to jen.Code for better type safety
				var fieldType jen.Code
				if field.Type != nil {
					fieldType = astTypeToJenCode(field.Type, resolver)
				} else {
					fieldType = jen.Interface()
				}

				// Generate appropriate methods based on field type
				if field.Type != nil {
					if isSliceOrArrayAST(field.Type) {
						writeSliceWithOptAST(buf, fieldName, field.Type, c, resolver)
						writeSliceSetOptAST(buf, fieldName, fieldType, c)
					} else if isMapAST(field.Type) {
						writeMapWithOptAST(buf, fieldName, field.Type, c, resolver)
						writeMapSetOptAST(buf, fieldName, fieldType, c)
					} else {
						writeStandardWithOptAST(buf, fieldName, fieldType, c)
					}
				} else {
					writeStandardWithOptAST(buf, fieldName, fieldType, c)
				}
			}
		}
	}
}

// writeSliceWithOptAST generates a With* method for slice fields using AST (appends)
func writeSliceWithOptAST(buf *jen.File, fieldName string, fieldTypeAST ast.Expr, c Config, resolver *ImportResolver) {
	fieldFuncName := fmt.Sprintf("With%s", strings.Title(fieldName))
	buf.Comment(fmt.Sprintf("%s returns an option that can append %ss to %s.%s", fieldFuncName, strings.Title(fieldName), c.StructName, fieldName))

	// Extract element type from slice/array AST
	var elemType jen.Code
	if arrayType, ok := fieldTypeAST.(*ast.ArrayType); ok {
		elemType = astTypeToJenCode(arrayType.Elt, resolver)
	} else {
		elemType = jen.Interface()
	}

	buf.Func().Id(fieldFuncName).Params(
		jen.Id(unexport(fieldName)).Add(elemType),
	).Id(c.OptTypeName).BlockFunc(func(grp *jen.Group) {
		grp.Return(
			jen.Func().Params(jen.Id(c.ReceiverId).Op("*").Add(c.StructRef...)).BlockFunc(func(grp2 *jen.Group) {
				grp2.Id(c.ReceiverId).Op(".").Id(fieldName).Op("=").Append(jen.Id(c.ReceiverId).Op(".").Id(fieldName), jen.Id(unexport(fieldName)))
			}),
		)
	})
}

// writeSliceSetOptAST generates a Set* method for slice fields using AST (replaces)
func writeSliceSetOptAST(buf *jen.File, fieldName string, fieldType jen.Code, c Config) {
	fieldFuncName := fmt.Sprintf("Set%s", strings.Title(fieldName))
	buf.Comment(fmt.Sprintf("%s returns an option that can set %s on a %s", fieldFuncName, strings.Title(fieldName), c.StructName))

	buf.Func().Id(fieldFuncName).Params(
		jen.Id(unexport(fieldName)).Add(fieldType),
	).Id(c.OptTypeName).BlockFunc(func(grp *jen.Group) {
		grp.Return(
			jen.Func().Params(jen.Id(c.ReceiverId).Op("*").Add(c.StructRef...)).BlockFunc(func(grp2 *jen.Group) {
				grp2.Id(c.ReceiverId).Op(".").Id(fieldName).Op("=").Id(unexport(fieldName))
			}),
		)
	})
}

// writeMapWithOptAST generates a With* method for map fields using AST (adds key-value)
func writeMapWithOptAST(buf *jen.File, fieldName string, fieldTypeAST ast.Expr, c Config, resolver *ImportResolver) {
	fieldFuncName := fmt.Sprintf("With%s", strings.Title(fieldName))
	buf.Comment(fmt.Sprintf("%s returns an option that can append %ss to %s.%s", fieldFuncName, strings.Title(fieldName), c.StructName, fieldName))

	// Extract key and value types from map AST
	var keyType, valueType jen.Code
	if mapType, ok := fieldTypeAST.(*ast.MapType); ok {
		keyType = astTypeToJenCode(mapType.Key, resolver)
		valueType = astTypeToJenCode(mapType.Value, resolver)
	} else {
		keyType = jen.Interface()
		valueType = jen.Interface()
	}

	buf.Func().Id(fieldFuncName).Params(
		jen.Id("key").Add(keyType),
		jen.Id("value").Add(valueType),
	).Id(c.OptTypeName).BlockFunc(func(grp *jen.Group) {
		grp.Return(
			jen.Func().Params(jen.Id(c.ReceiverId).Op("*").Add(c.StructRef...)).BlockFunc(func(grp2 *jen.Group) {
				grp2.Id(c.ReceiverId).Op(".").Id(fieldName).Index(jen.Id("key")).Op("=").Id("value")
			}),
		)
	})
}

// writeMapSetOptAST generates a Set* method for map fields using AST (replaces)
func writeMapSetOptAST(buf *jen.File, fieldName string, fieldType jen.Code, c Config) {
	fieldFuncName := fmt.Sprintf("Set%s", strings.Title(fieldName))
	buf.Comment(fmt.Sprintf("%s returns an option that can set %s on a %s", fieldFuncName, strings.Title(fieldName), c.StructName))

	buf.Func().Id(fieldFuncName).Params(
		jen.Id(unexport(fieldName)).Add(fieldType),
	).Id(c.OptTypeName).BlockFunc(func(grp *jen.Group) {
		grp.Return(
			jen.Func().Params(jen.Id(c.ReceiverId).Op("*").Add(c.StructRef...)).BlockFunc(func(grp2 *jen.Group) {
				grp2.Id(c.ReceiverId).Op(".").Id(fieldName).Op("=").Id(unexport(fieldName))
			}),
		)
	})
}

// writeStandardWithOptAST generates a With* method for standard fields using AST
func writeStandardWithOptAST(buf *jen.File, fieldName string, fieldType jen.Code, c Config) {
	fieldFuncName := fmt.Sprintf("With%s", strings.Title(fieldName))
	buf.Comment(fmt.Sprintf("%s returns an option that can set %s on a %s", fieldFuncName, strings.Title(fieldName), c.StructName))

	buf.Func().Id(fieldFuncName).Params(
		jen.Id(unexport(fieldName)).Add(fieldType),
	).Id(c.OptTypeName).BlockFunc(func(grp *jen.Group) {
		grp.Return(
			jen.Func().Params(jen.Id(c.ReceiverId).Op("*").Add(c.StructRef...)).BlockFunc(func(grp2 *jen.Group) {
				grp2.Id(c.ReceiverId).Op(".").Id(fieldName).Op("=").Id(unexport(fieldName))
			}),
		)
	})
}

// isSliceOrArrayAST checks if an AST type is a slice or array
func isSliceOrArrayAST(t ast.Expr) bool {
	_, ok := t.(*ast.ArrayType)
	return ok
}

// isMapAST checks if an AST type is a map
func isMapAST(t ast.Expr) bool {
	_, ok := t.(*ast.MapType)
	return ok
}

// astTypeToJenCode converts an AST type expression to jen.Code for code generation.
// It handles basic types, pointers, selectors, arrays, maps, interfaces, and channels.
func astTypeToJenCode(expr ast.Expr, resolver *ImportResolver) jen.Code {
	switch t := expr.(type) {
	case *ast.Ident:
		return jen.Id(t.Name)
	case *ast.StarExpr:
		return jen.Op("*").Add(astTypeToJenCode(t.X, resolver))
	case *ast.SelectorExpr:
		if pkg, ok := t.X.(*ast.Ident); ok {
			importPath := resolver.Resolve(pkg.Name)
			return jen.Qual(importPath, t.Sel.Name)
		}
		return jen.Interface()
	case *ast.ArrayType:
		if t.Len == nil {
			// slice
			return jen.Index().Add(astTypeToJenCode(t.Elt, resolver))
		}
		// array - for simplicity, treat as slice
		return jen.Index().Add(astTypeToJenCode(t.Elt, resolver))
	case *ast.MapType:
		return jen.Map(astTypeToJenCode(t.Key, resolver)).Add(astTypeToJenCode(t.Value, resolver))
	case *ast.InterfaceType:
		return jen.Interface()
	case *ast.ChanType:
		switch t.Dir {
		case ast.SEND:
			return jen.Op("<-").Chan().Add(astTypeToJenCode(t.Value, resolver))
		case ast.RECV:
			return jen.Chan().Op("<-").Add(astTypeToJenCode(t.Value, resolver))
		default:
			return jen.Chan().Add(astTypeToJenCode(t.Value, resolver))
		}
	default:
		// Fallback to interface{} for unknown types
		return jen.Interface()
	}
}

func applyOptions(receiverId string) func(grp *jen.Group) {
	return func(grp *jen.Group) {
		grp.For(jen.Id("_").Op(",").Id("o").Op(":=").Op("range").Id("opts")).Block(
			jen.Id("o").Params(jen.Id(receiverId)),
		)
		grp.Return(jen.Id(receiverId))
	}
}

func unexport(s string) string {
	if len(s) == 0 {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToLower(r[0])
	return string(r)
}
