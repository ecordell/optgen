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
//	-prefix
//	    Prefix generated function names with struct name (e.g., WithServerPort instead of WithPort)
//	-flatten
//	    Generate flattened accessor methods for nested struct fields
//
// Example:
//
//	//go:generate go run github.com/ecordell/optgen -output=config_options.go . Config
//
// Struct Tag Format:
//
// Fields must be annotated with the `debugmap` struct tag:
//   - "visible" - Show actual field value in DebugMap
//   - "visible-format" - Show formatted value (expands collections, inlines nested structs)
//   - "sensitive" - Show "(sensitive)" placeholder
//   - "hidden" - Omit from DebugMap entirely
//
// Fields can optionally be annotated with the `optgen` struct tag:
//   - "generate" - Generate With* functions (default for exported fields)
//   - "skip" - Don't generate any functions
//   - "readonly" - Only include in ToOption(), no With* functions
//   - "generate,recursive" - For struct fields: generate both direct setter and options setter
//   - "generate,flatten" - Generate flattened accessors for nested struct fields (unlimited depth)
//   - "generate,flatten:N" - Flatten with depth limit N
//   - "generate,flatten,prefix:Custom" - Use custom prefix for flattened names
//   - "generate,public" / "generate,private" - Override visibility
//
// Example struct:
//
//	type ServerMetadata struct {
//	    Name  string `optgen:"generate" debugmap:"visible"`
//	    Owner string `optgen:"generate" debugmap:"visible"`
//	}
//
//	type Config struct {
//	    Name     string          `optgen:"generate" debugmap:"visible"`
//	    Password string          `optgen:"generate" debugmap:"sensitive"`
//	    Data     []byte          `optgen:"skip" debugmap:"hidden"`
//	    Metadata ServerMetadata  `optgen:"generate,recursive" debugmap:"visible"`
//	    Address  Address         `optgen:"generate,flatten:1" debugmap:"visible"`
//	}
//
// Generated functions for the above Config:
//   - WithName(name string) - standard setter
//   - WithPassword(password string) - standard setter
//   - WithMetadata(metadata ServerMetadata) - direct struct setter
//   - WithMetadataOptions(opts ...ServerMetadataOption) - nested options setter (recursive)
//   - WithAddress(address Address) - direct struct setter
//   - WithAddressStreet(street string) - flattened accessor (depth 1)
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
	"strconv"
	"strings"
	"unicode"

	_ "github.com/creasty/defaults"
	"github.com/dave/jennifer/jen"
	"github.com/fatih/structtag"
)

type WriterProvider func() io.Writer


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
	prefixFlag := fs.Bool(
		"prefix",
		false,
		"Prefix generated function names with struct name (e.g., WithServerPort instead of WithPort)",
	)
	flattenFlag := fs.Bool(
		"flatten",
		false,
		"Generate flattened accessor methods for nested struct fields",
	)

	if err := fs.Parse(os.Args[1:]); err != nil {
		log.Fatal(err.Error())
	}

	if len(fs.Args()) < 2 {
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

	// Determine package name from output directory or flag
	packageName := func() string {
		if pkgNameFlag != nil && *pkgNameFlag != "" {
			return *pkgNameFlag
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
		fset := token.NewFileSet()
		pkgs, err := parser.ParseDir(fset, pkgName, nil, parser.ParseComments)
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
				err = generateForFileAST(f, structs, packageName, f.Name.Name, *outputPathFlag, sensitiveNameMatches, *prefixFlag, *flattenFlag, writer)
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
	UsePrefix      bool
	UseFlatten     bool
}

// prefix returns the struct name if UsePrefix is true, otherwise empty string
func (c Config) prefix() string {
	if c.UsePrefix {
		return c.StructName
	}
	return ""
}

const (
	DebugMapFieldTag = "debugmap"
	OptgenFieldTag   = "optgen"

	// OptgenFieldTag values
	OptgenGenerate = "generate" // Default behavior
	OptgenSkip     = "skip"     // Don't generate options
	OptgenReadonly = "readonly" // Only in constructor

	// Type categories for debug code generation
	typeCategoryPrimitive = "primitive"
	typeCategoryPointer   = "pointer"
	typeCategorySlice     = "slice"
	typeCategoryMap       = "map"
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

// OptgenTagInfo contains parsed optgen tag information
type OptgenTagInfo struct {
	Action        string // "generate", "skip", "readonly"
	Visibility    string // "public", "private", "default"
	Recursive     bool   // true if "recursive" present
	Flatten       bool   // true if "flatten" present
	FlattenDepth  int    // 0 = unlimited, >0 = specific depth
	FlattenPrefix string // custom prefix for flattened names, empty = use field name
}

// parseOptgenTag parses the optgen struct tag value.
// Returns tag info including action and visibility.
// If tag doesn't exist, returns default behavior based on field visibility.
func parseOptgenTag(field *ast.Field) (OptgenTagInfo, bool) {
	if field.Names == nil {
		return OptgenTagInfo{Action: OptgenSkip, Visibility: "default"}, false
	}

	isExported := field.Names[0].IsExported()

	tagValue, err := parseStructTag(field, OptgenFieldTag)
	if err != nil {
		// No tag present - use default behavior
		action := OptgenSkip
		if isExported {
			action = OptgenGenerate
		}
		return OptgenTagInfo{Action: action, Visibility: "default"}, false
	}

	// Parse comma-separated values: "generate,public,recursive,flatten:2,prefix:Custom"
	parts := strings.Split(tagValue, ",")
	info := OptgenTagInfo{
		Action:        strings.TrimSpace(parts[0]),
		Visibility:    "default",
		Recursive:     false,
		Flatten:       false,
		FlattenDepth:  0,
		FlattenPrefix: "",
	}

	// Validate action
	switch info.Action {
	case OptgenGenerate, OptgenSkip, OptgenReadonly:
		// Valid
	default:
		fmt.Printf("unknown optgen action '%s' on field %s\n", info.Action, field.Names[0].Name)
		os.Exit(1)
	}

	// Parse additional options (visibility, recursive, flatten, etc.)
	for i := 1; i < len(parts); i++ {
		part := strings.TrimSpace(parts[i])

		// Check for key:value options
		if strings.Contains(part, ":") {
			kv := strings.SplitN(part, ":", 2)
			key := strings.TrimSpace(kv[0])
			value := strings.TrimSpace(kv[1])

			switch key {
			case "flatten":
				// Parse flatten depth: "flatten:2"
				info.Flatten = true
				depth, err := strconv.Atoi(value)
				if err != nil || depth < 0 {
					fmt.Printf("invalid flatten depth '%s' on field %s\n", value, field.Names[0].Name)
					os.Exit(1)
				}
				info.FlattenDepth = depth
			case "prefix":
				// Parse custom prefix: "prefix:Custom"
				info.FlattenPrefix = value
			default:
				fmt.Printf("unknown optgen option '%s' on field %s\n", key, field.Names[0].Name)
				os.Exit(1)
			}
		} else {
			// Simple flags
			switch part {
			case "public", "private":
				info.Visibility = part
			case "recursive":
				info.Recursive = true
			case "flatten":
				info.Flatten = true
				info.FlattenDepth = 0 // unlimited
			default:
				fmt.Printf("unknown optgen option '%s' on field %s\n", part, field.Names[0].Name)
				os.Exit(1)
			}
		}
	}

	return info, true
}

// generateForFileAST generates functional options code for the given struct types.
// It creates option types, constructor functions, and utility methods for each struct.
func generateForFileAST(file *ast.File, typeSpecs []*ast.TypeSpec, pkgName, fileName, outpath string, sensitiveNameMatches []string, usePrefix, useFlatten bool, writer WriterProvider) error {
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
			TargetTypeName: toTitle(structName),
			StructRef:      []jen.Code{jen.Id(structName)},
			StructName:     structName,
			PkgPath:        "", // Not needed for AST-based generation
			UsePrefix:      usePrefix,
			UseFlatten:     useFlatten,
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
		writeDebugMapAST(buf, st, config, sensitiveNameMatches, resolver)

		// generate WithOptions
		writeXWithOptionsAST(buf, config)
		writeWithOptionsAST(buf, config)

		// generate all With* functions
		writeAllWithOptFuncsAST(buf, st, outdir, config, resolver, file)
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
	newFuncName := "ToOption"

	buf.Comment(fmt.Sprintf("%s returns a new %s that sets the values from the passed in %s", newFuncName, c.OptTypeName, c.StructName))
	buf.Func().Params(jen.Id(c.ReceiverId).Op("*").Id(c.StructName)).Id(newFuncName).Params().Id(c.OptTypeName).BlockFunc(func(grp *jen.Group) {
		grp.Return(jen.Func().Params(jen.Id("to").Op("*").Id(c.StructName)).BlockFunc(func(retGrp *jen.Group) {
			for _, field := range st.Fields.List {
				for _, name := range field.Names {
					if name.IsExported() {
						// Check optgen tag
						tagInfo, _ := parseOptgenTag(field)
						if tagInfo.Action == OptgenSkip {
							continue
						}
						// readonly fields ARE included here
						retGrp.Id("to").Op(".").Id(name.Name).Op("=").Id(c.ReceiverId).Op(".").Id(name.Name)
					}
				}
			}
		}))
	})
}

func writeDebugMapAST(buf *jen.File, st *ast.StructType, c Config, sensitiveNameMatches []string, resolver *ImportResolver) {
	newFuncName := "DebugMap"

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

				processDebugMapField(grp, field, name.Name, c, sensitiveNameMatches, mapId, resolver)
			}
		}

		grp.Return(jen.Id(mapId))
	})

	// Generate FlatDebugMap method
	writeFlatDebugMapAST(buf, c)
}

// writeFlatDebugMapAST generates a FlatDebugMap method that flattens nested maps inline
func writeFlatDebugMapAST(buf *jen.File, c Config) {
	buf.Comment(fmt.Sprintf("FlatDebugMap returns a flattened map form of %s for debugging", c.TargetTypeName))
	buf.Comment("Nested maps are flattened using dot notation (e.g., \"parent.child.field\")")
	buf.Func().Params(jen.Id(c.ReceiverId).Op("*").Id(c.StructName)).Id("FlatDebugMap").Params().Id("map[string]any").BlockFunc(func(grp *jen.Group) {
		// Define a recursive anonymous function to flatten maps
		grp.Var().Id("flatten").Func().Params(
			jen.Id("m").Map(jen.String()).Any(),
		).Map(jen.String()).Any()

		grp.Id("flatten").Op("=").Func().Params(
			jen.Id("m").Map(jen.String()).Any(),
		).Map(jen.String()).Any().BlockFunc(func(fnGrp *jen.Group) {
			fnGrp.Id("result").Op(":=").Make(jen.Map(jen.String()).Any(), jen.Len(jen.Id("m")))
			fnGrp.For(jen.List(jen.Id("key"), jen.Id("value")).Op(":=").Range().Id("m")).BlockFunc(func(forGrp *jen.Group) {
				forGrp.List(jen.Id("childMap"), jen.Id("ok")).Op(":=").Id("value").Assert(jen.Map(jen.String()).Any())
				forGrp.If(jen.Id("ok")).BlockFunc(func(ifGrp *jen.Group) {
					ifGrp.For(jen.List(jen.Id("childKey"), jen.Id("childValue")).Op(":=").Range().Id("flatten").Call(jen.Id("childMap"))).Block(
						jen.Id("result").Index(jen.Id("key").Op("+").Lit(".").Op("+").Id("childKey")).Op("=").Id("childValue"),
					)
					ifGrp.Continue()
				})
				forGrp.Id("result").Index(jen.Id("key")).Op("=").Id("value")
			})
			fnGrp.Return(jen.Id("result"))
		})

		grp.Return(jen.Id("flatten").Call(jen.Id(c.ReceiverId).Dot("DebugMap").Call()))
	})
}

// processDebugMapField processes a single field for debug map generation
func processDebugMapField(grp *jen.Group, field *ast.Field, fieldName string, c Config, sensitiveNameMatches []string, mapId string, resolver *ImportResolver) {
	// Parse the debugmap tag
	tagValue, err := parseStructTag(field, DebugMapFieldTag)
	if err != nil {
		fmt.Printf("missing debugmap tag on field %s in type %s\n", fieldName, c.TargetTypeName)
		os.Exit(1)
	}

	switch tagValue {
	case "visible":
		validateNotSensitive(fieldName, c.TargetTypeName, sensitiveNameMatches)
		generateDebugCodeByCategory(grp, field.Type, c.ReceiverId, fieldName, mapId, false, resolver)

	case "visible-format":
		validateNotSensitive(fieldName, c.TargetTypeName, sensitiveNameMatches)
		generateDebugCodeByCategory(grp, field.Type, c.ReceiverId, fieldName, mapId, true, resolver)

	case "hidden":
		// Skip this field entirely
		return

	case "sensitive":
		category := getTypeCategory(field.Type)
		generateDebugCodeForSensitive(grp, c.ReceiverId, fieldName, field.Type, category, mapId)

	default:
		fmt.Printf("unknown value '%s' for debugmap tag on field %s in type %s\n", tagValue, fieldName, c.TargetTypeName)
		os.Exit(1)
	}
}

// validateNotSensitive checks that a field name doesn't contain sensitive patterns
func validateNotSensitive(fieldName, typeName string, sensitiveNameMatches []string) {
	for _, sensitiveName := range sensitiveNameMatches {
		if strings.Contains(strings.ToLower(fieldName), sensitiveName) {
			fmt.Printf("field %s in type %s must be marked as 'sensitive'\n", fieldName, typeName)
			os.Exit(1)
		}
	}
}

// generateDebugCodeByCategory generates debug code based on type category
func generateDebugCodeByCategory(grp *jen.Group, fieldType ast.Expr, receiverId, fieldName, mapId string, useFormat bool, resolver *ImportResolver) {
	category := getTypeCategory(fieldType)

	// Check if it's a struct type
	isStruct, pkgPath := isStructTypeAST(fieldType, resolver)
	if isStruct {
		if pkgPath == "" {
			// Same-package struct - call DebugMap() method
			if useFormat {
				// For visible-format, inline the nested fields with dot notation
				generateDebugCodeForStructFormat(grp, receiverId, fieldName, mapId)
			} else {
				// For visible, call DebugMap() recursively
				generateDebugCodeForStruct(grp, receiverId, fieldName, mapId)
			}
		} else {
			// Cross-package struct - just use fmt.Sprintf
			grp.Id(mapId).Index(jen.Lit(fieldName)).Op("=").Qual("fmt", "Sprintf").Call(
				jen.Lit("%v"),
				jen.Id(receiverId).Dot(fieldName),
			)
		}
		return
	}

	switch category {
	case typeCategoryPrimitive:
		generateDebugCodeForPrimitive(grp, receiverId, fieldName, fieldType, mapId)
	case typeCategoryPointer:
		generateDebugCodeForPointer(grp, receiverId, fieldName, fieldType, mapId)
	case typeCategorySlice:
		if useFormat {
			generateDebugCodeForSliceFormat(grp, receiverId, fieldName, fieldType, mapId)
		} else {
			generateDebugCodeForSliceSize(grp, receiverId, fieldName, mapId)
		}
	case typeCategoryMap:
		if useFormat {
			generateDebugCodeForMapFormat(grp, receiverId, fieldName, mapId)
		} else {
			generateDebugCodeForMapSize(grp, receiverId, fieldName, mapId)
		}
	default:
		// Complex types: direct assignment
		grp.Id(mapId).Index(jen.Lit(fieldName)).Op("=").Id(receiverId).Dot(fieldName)
	}
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

func writeAllWithOptFuncsAST(buf *jen.File, st *ast.StructType, outdir string, c Config, resolver *ImportResolver, file *ast.File) {
	for _, field := range st.Fields.List {
		if field.Names == nil {
			// Anonymous field, skip
			continue
		}

		for _, name := range field.Names {
			fieldName := name.Name
			isExported := name.IsExported()

			// Check optgen tag
			tagInfo, _ := parseOptgenTag(field)

			// Skip fields marked as skip or readonly
			if tagInfo.Action == OptgenSkip || tagInfo.Action == OptgenReadonly {
				continue
			}

			// Determine function visibility
			makePublic := isExported // Default: match field visibility
			if tagInfo.Visibility == "public" {
				makePublic = true
			} else if tagInfo.Visibility == "private" {
				makePublic = false
			}

			// Try to convert AST type to jen.Code for better type safety
			var fieldType jen.Code
			if field.Type != nil {
				fieldType = astTypeToJenCode(field.Type, resolver)
			} else {
				fieldType = jen.Interface()
			}

			// Generate appropriate methods based on field type
			if field.Type != nil {
				// Check if it's a struct type
				isStruct, pkgPath := isStructTypeAST(field.Type, resolver)
				if isStruct && pkgPath == "" {
					// Same-package struct type
					writeStructDirectSetterAST(buf, fieldName, fieldType, c, makePublic)

					// Generate recursive options setter if requested
					if tagInfo.Recursive {
						writeStructRecursiveSetterAST(buf, fieldName, field.Type, c, resolver, makePublic)
					}

					// Generate flattened accessors if requested (via tag or global flag)
					if tagInfo.Flatten || c.UseFlatten {
						flattenDepth := tagInfo.FlattenDepth
						flattenPrefix := tagInfo.FlattenPrefix
						if flattenPrefix == "" {
							flattenPrefix = fieldName
						}
						writeFlattenedOptFuncsAST(buf, fieldName, field.Type, file, c, resolver, flattenPrefix, 1, flattenDepth, makePublic)
					}
				} else if isSliceOrArrayAST(field.Type) {
					writeSliceWithOptAST(buf, fieldName, field.Type, c, resolver, makePublic)
					writeSliceSetOptAST(buf, fieldName, fieldType, c, makePublic)
				} else if isMapAST(field.Type) {
					writeMapWithOptAST(buf, fieldName, field.Type, c, resolver, makePublic)
					writeMapSetOptAST(buf, fieldName, fieldType, c, makePublic)
				} else {
					writeStandardWithOptAST(buf, fieldName, fieldType, c, makePublic)
				}
			} else {
				writeStandardWithOptAST(buf, fieldName, fieldType, c, makePublic)
			}
		}
	}
}

// writeSliceWithOptAST generates a With* method for slice fields using AST (appends)
func writeSliceWithOptAST(buf *jen.File, fieldName string, fieldTypeAST ast.Expr, c Config, resolver *ImportResolver, makePublic bool) {
	fieldFuncName := formatFunctionName("With", fieldName, c.prefix(), makePublic)
	buf.Comment(fmt.Sprintf("%s returns an option that can append %ss to %s.%s", fieldFuncName, toTitle(fieldName), c.StructName, toTitle(fieldName)))

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
				grp2.Id(c.ReceiverId).Op(".").Id(toTitle(fieldName)).Op("=").Append(jen.Id(c.ReceiverId).Op(".").Id(toTitle(fieldName)), jen.Id(unexport(fieldName)))
			}),
		)
	})
}

// writeSliceSetOptAST generates a Set* method for slice fields using AST (replaces)
func writeSliceSetOptAST(buf *jen.File, fieldName string, fieldType jen.Code, c Config, makePublic bool) {
	writeSetterOptAST(buf, "Set", fieldName, fieldType, c, makePublic)
}

// writeMapWithOptAST generates a With* method for map fields using AST (adds key-value)
func writeMapWithOptAST(buf *jen.File, fieldName string, fieldTypeAST ast.Expr, c Config, resolver *ImportResolver, makePublic bool) {
	fieldFuncName := formatFunctionName("With", fieldName, c.prefix(), makePublic)
	buf.Comment(fmt.Sprintf("%s returns an option that can append %ss to %s.%s", fieldFuncName, toTitle(fieldName), c.StructName, toTitle(fieldName)))

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
				grp2.Id(c.ReceiverId).Op(".").Id(toTitle(fieldName)).Index(jen.Id("key")).Op("=").Id("value")
			}),
		)
	})
}

// writeMapSetOptAST generates a Set* method for map fields using AST (replaces)
func writeMapSetOptAST(buf *jen.File, fieldName string, fieldType jen.Code, c Config, makePublic bool) {
	writeSetterOptAST(buf, "Set", fieldName, fieldType, c, makePublic)
}

// writeStandardWithOptAST generates a With* method for standard fields using AST
func writeStandardWithOptAST(buf *jen.File, fieldName string, fieldType jen.Code, c Config, makePublic bool) {
	writeSetterOptAST(buf, "With", fieldName, fieldType, c, makePublic)
}

// writeStructDirectSetterAST generates a With* method for struct fields (direct assignment)
func writeStructDirectSetterAST(buf *jen.File, fieldName string, fieldType jen.Code, c Config, makePublic bool) {
	writeSetterOptAST(buf, "With", fieldName, fieldType, c, makePublic)
}

// writeStructRecursiveSetterAST generates a WithFieldOptions method for struct fields (nested options)
func writeStructRecursiveSetterAST(buf *jen.File, fieldName string, fieldTypeAST ast.Expr, c Config, resolver *ImportResolver, makePublic bool) {
	// Get the struct type name
	typeName := getStructTypeName(fieldTypeAST)
	if typeName == "" {
		return // Can't generate without a type name
	}

	// Generate function name: WithMetadataOptions
	fieldFuncName := formatFunctionName("With", fieldName+"Options", c.prefix(), makePublic)
	optTypeName := fmt.Sprintf("%sOption", typeName)

	buf.Comment(fmt.Sprintf("%s returns an option that can set %s on a %s using nested options", fieldFuncName, toTitle(fieldName), c.StructName))
	buf.Func().Id(fieldFuncName).Params(
		jen.Id("opts").Op("...").Id(optTypeName),
	).Id(c.OptTypeName).BlockFunc(func(grp *jen.Group) {
		grp.Return(
			jen.Func().Params(jen.Id(c.ReceiverId).Op("*").Add(c.StructRef...)).BlockFunc(func(grp2 *jen.Group) {
				// Call New{Type}WithOptions(opts...)
				constructorName := fmt.Sprintf("New%sWithOptions", typeName)
				grp2.Id(c.ReceiverId).Op(".").Id(toTitle(fieldName)).Op("=").Op("*").Id(constructorName).Call(jen.Id("opts").Op("..."))
			}),
		)
	})
}

// writeFlattenedOptFuncsAST generates flattened accessor methods for nested struct fields
func writeFlattenedOptFuncsAST(buf *jen.File, parentFieldName string, fieldTypeAST ast.Expr, file *ast.File, c Config, resolver *ImportResolver, prefix string, currentDepth, maxDepth int, makePublic bool) {
	// Check depth limit
	if maxDepth > 0 && currentDepth > maxDepth {
		return
	}

	// Get the struct type name and look it up
	typeName := getStructTypeName(fieldTypeAST)
	if typeName == "" {
		return
	}

	nestedStruct := findStructDefInFile(file, typeName)
	if nestedStruct == nil {
		return // Struct not found in file
	}

	// Generate flattened accessors for each field in the nested struct
	for _, nestedField := range nestedStruct.Fields.List {
		if nestedField.Names == nil {
			continue
		}

		for _, nestedName := range nestedField.Names {
			nestedFieldName := nestedName.Name

			// Skip unexported fields
			if !nestedName.IsExported() {
				continue
			}

			// Check optgen tag
			nestedTagInfo, _ := parseOptgenTag(nestedField)
			if nestedTagInfo.Action == OptgenSkip || nestedTagInfo.Action == OptgenReadonly {
				continue
			}

			// Generate function name with prefix: WithMetadataName
			flatFieldName := toTitle(prefix) + toTitle(nestedFieldName)
			fieldFuncName := formatFunctionName("With", flatFieldName, c.prefix(), makePublic)

			// Convert nested field type to jen.Code
			var nestedFieldType jen.Code
			if nestedField.Type != nil {
				nestedFieldType = astTypeToJenCode(nestedField.Type, resolver)
			} else {
				nestedFieldType = jen.Interface()
			}

			// Generate the setter function
			buf.Comment(fmt.Sprintf("%s returns an option that can set %s.%s on a %s", fieldFuncName, toTitle(parentFieldName), toTitle(nestedFieldName), c.StructName))
			buf.Func().Id(fieldFuncName).Params(
				jen.Id(unexport(nestedFieldName)).Add(nestedFieldType),
			).Id(c.OptTypeName).BlockFunc(func(grp *jen.Group) {
				grp.Return(
					jen.Func().Params(jen.Id(c.ReceiverId).Op("*").Add(c.StructRef...)).BlockFunc(func(grp2 *jen.Group) {
						grp2.Id(c.ReceiverId).Op(".").Id(toTitle(parentFieldName)).Op(".").Id(toTitle(nestedFieldName)).Op("=").Id(unexport(nestedFieldName))
					}),
				)
			})

			// Recursively flatten if this nested field is also a struct
			if nestedField.Type != nil {
				isNestedStruct, nestedPkgPath := isStructTypeAST(nestedField.Type, resolver)
				if isNestedStruct && nestedPkgPath == "" {
					// Recursively flatten this nested struct
					newPrefix := toTitle(prefix) + toTitle(nestedFieldName)
					newParentPath := parentFieldName + "." + nestedFieldName
					writeFlattenedOptFuncsAST(buf, newParentPath, nestedField.Type, file, c, resolver, newPrefix, currentDepth+1, maxDepth, makePublic)
				}
			}
		}
	}
}

// writeSetterOptAST generates a setter option function (used by slice, map, and standard setters)
func writeSetterOptAST(buf *jen.File, funcPrefix, fieldName string, fieldType jen.Code, c Config, makePublic bool) {
	fieldFuncName := formatFunctionName(funcPrefix, fieldName, c.prefix(), makePublic)
	buf.Comment(fmt.Sprintf("%s returns an option that can set %s on a %s", fieldFuncName, toTitle(fieldName), c.StructName))

	buf.Func().Id(fieldFuncName).Params(
		jen.Id(unexport(fieldName)).Add(fieldType),
	).Id(c.OptTypeName).BlockFunc(func(grp *jen.Group) {
		grp.Return(
			jen.Func().Params(jen.Id(c.ReceiverId).Op("*").Add(c.StructRef...)).BlockFunc(func(grp2 *jen.Group) {
				grp2.Id(c.ReceiverId).Op(".").Id(toTitle(fieldName)).Op("=").Id(unexport(fieldName))
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
// It handles basic types, pointers, selectors, arrays, maps, interfaces, channels, and generics.
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
	case *ast.IndexExpr:
		// Generic type with single type parameter: Type[T]
		base := astTypeToJenCode(t.X, resolver)
		typeParam := astTypeToJenCode(t.Index, resolver)
		// Index() with types creates Type[T] syntax
		return jen.Add(base).Types(typeParam)
	case *ast.IndexListExpr:
		// Generic type with multiple type parameters: Type[T, U, V]
		base := astTypeToJenCode(t.X, resolver)
		var params []jen.Code
		for _, index := range t.Indices {
			params = append(params, astTypeToJenCode(index, resolver))
		}
		// Types() with multiple params creates Type[T, U, V] syntax
		return jen.Add(base).Types(params...)
	default:
		// Fallback to interface{} for unknown types
		return jen.Interface()
	}
}

// getTypeCategory returns the category of a type for debug generation
func getTypeCategory(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		switch t.Name {
		case "string", "int", "int8", "int16", "int32", "int64",
			"uint", "uint8", "uint16", "uint32", "uint64",
			"bool", "float32", "float64":
			return typeCategoryPrimitive
		default:
			return "complex"
		}
	case *ast.StarExpr:
		return typeCategoryPointer
	case *ast.ArrayType:
		if t.Len == nil {
			return typeCategorySlice
		}
		return "array"
	case *ast.MapType:
		return typeCategoryMap
	default:
		return "complex"
	}
}

// isSamePackageStruct checks if an AST expression is a same-package struct type (not cross-package).
// Returns true for simple identifiers (like "ServerMetadata"), false for selectors (like "time.Time").
func isSamePackageStruct(expr ast.Expr) bool {
	// Unwrap pointer types
	if starExpr, ok := expr.(*ast.StarExpr); ok {
		expr = starExpr.X
	}

	// Check if it's a simple identifier (same package) or selector (cross-package)
	switch t := expr.(type) {
	case *ast.Ident:
		// Simple identifier - could be a struct type in the same package
		// We can't definitively know if it's a struct without looking it up,
		// but we return true to indicate it's a same-package type
		switch t.Name {
		case "string", "int", "int8", "int16", "int32", "int64",
			"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
			"bool", "float32", "float64", "complex64", "complex128",
			"byte", "rune", "error", "any":
			// Built-in types - not structs
			return false
		default:
			// Could be a struct type
			return true
		}
	case *ast.SelectorExpr:
		// Cross-package type (e.g., time.Time)
		return false
	case *ast.IndexExpr, *ast.IndexListExpr:
		// Generic type - not a simple struct
		return false
	default:
		return false
	}
}

// isStructTypeAST checks if an AST expression represents a struct type.
// Returns (isStruct, packagePath) where:
//   - isStruct is true if the type could be a struct
//   - packagePath is empty for same-package types, non-empty for cross-package (e.g., "time")
func isStructTypeAST(expr ast.Expr, resolver *ImportResolver) (bool, string) {
	// Unwrap pointer types
	if starExpr, ok := expr.(*ast.StarExpr); ok {
		expr = starExpr.X
	}

	switch t := expr.(type) {
	case *ast.Ident:
		// Simple identifier - check if it's a built-in type
		switch t.Name {
		case "string", "int", "int8", "int16", "int32", "int64",
			"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
			"bool", "float32", "float64", "complex64", "complex128",
			"byte", "rune", "error", "any":
			// Built-in types - not structs
			return false, ""
		default:
			// Could be a same-package struct type
			return true, ""
		}
	case *ast.SelectorExpr:
		// Cross-package type (e.g., time.Time, sql.NullString)
		if pkg, ok := t.X.(*ast.Ident); ok {
			importPath := resolver.Resolve(pkg.Name)
			return true, importPath
		}
		return false, ""
	case *ast.IndexExpr, *ast.IndexListExpr:
		// Generic type - not a simple struct for our purposes
		return false, ""
	default:
		return false, ""
	}
}

// findStructDefInFile searches for a struct type definition by name in an AST file.
// Returns the struct type if found, nil otherwise.
func findStructDefInFile(file *ast.File, typeName string) *ast.StructType {
	var result *ast.StructType

	ast.Inspect(file, func(node ast.Node) bool {
		if result != nil {
			return false // Already found, stop searching
		}

		ts, ok := node.(*ast.TypeSpec)
		if !ok {
			return true
		}

		if ts.Name == nil || ts.Name.Name != typeName {
			return true
		}

		// Check if it's a struct type
		if st, isStruct := ts.Type.(*ast.StructType); isStruct {
			result = st
			return false
		}

		return true
	})

	return result
}

// getStructTypeName extracts the type name from an AST expression.
// Returns the type name (e.g., "ServerMetadata" from *ast.Ident, or "Time" from time.Time selector)
func getStructTypeName(expr ast.Expr) string {
	// Unwrap pointer types
	if starExpr, ok := expr.(*ast.StarExpr); ok {
		expr = starExpr.X
	}

	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return t.Sel.Name
	default:
		return ""
	}
}

// isStringType checks if a type is a string
func isStringType(expr ast.Expr) bool {
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name == "string"
	}
	return false
}

// getSliceElementType returns the element type of a slice/array
func getSliceElementType(expr ast.Expr) ast.Expr {
	if arrayType, ok := expr.(*ast.ArrayType); ok {
		return arrayType.Elt
	}
	return nil
}

// generateDebugCodeForPrimitive handles primitive types (string, int, bool, float)
func generateDebugCodeForPrimitive(grp *jen.Group, receiverId, fieldName string, fieldType ast.Expr, mapId string) {
	fieldAccess := jen.Id(receiverId).Dot(fieldName)

	if isStringType(fieldType) {
		// String: check for empty
		grp.If(jen.Add(fieldAccess).Op("==").Lit("")).Block(
			jen.Id(mapId).Index(jen.Lit(fieldName)).Op("=").Lit("(empty)"),
		).Else().Block(
			jen.Id(mapId).Index(jen.Lit(fieldName)).Op("=").Add(fieldAccess),
		)
	} else {
		// Other primitives: direct assignment
		grp.Id(mapId).Index(jen.Lit(fieldName)).Op("=").Add(fieldAccess)
	}
}

// generateDebugCodeForPointer handles pointer types
func generateDebugCodeForPointer(grp *jen.Group, receiverId, fieldName string, fieldType ast.Expr, mapId string) {
	fieldAccess := jen.Id(receiverId).Dot(fieldName)

	// Check for nil, then dereference
	grp.If(jen.Add(fieldAccess).Op("==").Nil()).Block(
		jen.Id(mapId).Index(jen.Lit(fieldName)).Op("=").Lit("nil"),
	).Else().Block(
		jen.Id(mapId).Index(jen.Lit(fieldName)).Op("=").Op("*").Add(fieldAccess),
	)
}

// generateDebugCodeForSliceSize generates code for slice with size display (visible tag)
func generateDebugCodeForSliceSize(grp *jen.Group, receiverId, fieldName, mapId string) {
	generateDebugCodeForCollectionSize(grp, receiverId, fieldName, mapId, "slice")
}

// generateDebugCodeForSliceFormat generates code for slice with expanded values (visible-format tag)
func generateDebugCodeForSliceFormat(grp *jen.Group, receiverId, fieldName string, fieldType ast.Expr, mapId string) {
	fieldAccess := jen.Id(receiverId).Dot(fieldName)
	elemType := getSliceElementType(fieldType)
	debugVarName := "debug" + fieldName

	grp.If(jen.Add(fieldAccess).Op("==").Nil()).Block(
		jen.Id(mapId).Index(jen.Lit(fieldName)).Op("=").Lit("nil"),
	).Else().Block(
		jen.Id(debugVarName).Op(":=").Make(jen.Index().Any(), jen.Lit(0), jen.Len(fieldAccess)),
		jen.For(jen.List(jen.Id("_"), jen.Id("v")).Op(":=").Range().Add(fieldAccess)).BlockFunc(func(forGrp *jen.Group) {
			if elemType != nil && isStringType(elemType) {
				// String slice: check for empty strings
				forGrp.If(jen.Id("v").Op("==").Lit("")).Block(
					jen.Id(debugVarName).Op("=").Append(jen.Id(debugVarName), jen.Lit("(empty)")),
				).Else().Block(
					jen.Id(debugVarName).Op("=").Append(jen.Id(debugVarName), jen.Id("v")),
				)
			} else {
				// Other types: direct append
				forGrp.Id(debugVarName).Op("=").Append(jen.Id(debugVarName), jen.Id("v"))
			}
		}),
		jen.Id(mapId).Index(jen.Lit(fieldName)).Op("=").Id(debugVarName),
	)
}

// generateDebugCodeForMapSize generates code for map with size display (visible tag)
func generateDebugCodeForMapSize(grp *jen.Group, receiverId, fieldName, mapId string) {
	generateDebugCodeForCollectionSize(grp, receiverId, fieldName, mapId, "map")
}

// generateDebugCodeForCollectionSize generates code for slice/map with size display
func generateDebugCodeForCollectionSize(grp *jen.Group, receiverId, fieldName, mapId, collectionType string) {
	fieldAccess := jen.Id(receiverId).Dot(fieldName)

	grp.If(jen.Add(fieldAccess).Op("==").Nil()).Block(
		jen.Id(mapId).Index(jen.Lit(fieldName)).Op("=").Lit("nil"),
	).Else().Block(
		jen.Id(mapId).Index(jen.Lit(fieldName)).Op("=").Qual("fmt", "Sprintf").Call(
			jen.Lit(fmt.Sprintf("(%s of size %%d)", collectionType)),
			jen.Len(fieldAccess),
		),
	)
}

// generateDebugCodeForMapFormat generates code for map with expanded values (visible-format tag)
func generateDebugCodeForMapFormat(grp *jen.Group, receiverId, fieldName, mapId string) {
	fieldAccess := jen.Id(receiverId).Dot(fieldName)

	grp.If(jen.Add(fieldAccess).Op("==").Nil()).Block(
		jen.Id(mapId).Index(jen.Lit(fieldName)).Op("=").Lit("nil"),
	).Else().Block(
		jen.Id(mapId).Index(jen.Lit(fieldName)).Op("=").Qual("fmt", "Sprintf").Call(
			jen.Lit("%v"),
			fieldAccess,
		),
	)
}

// generateDebugCodeForSensitive generates code for sensitive fields
func generateDebugCodeForSensitive(grp *jen.Group, receiverId, fieldName string, fieldType ast.Expr, category, mapId string) {
	fieldAccess := jen.Id(receiverId).Dot(fieldName)

	if category == typeCategoryPointer {
		// Pointer: check nil first
		grp.If(jen.Add(fieldAccess).Op("==").Nil()).Block(
			jen.Id(mapId).Index(jen.Lit(fieldName)).Op("=").Lit("nil"),
		).Else().Block(
			jen.Id(mapId).Index(jen.Lit(fieldName)).Op("=").Lit("(sensitive)"),
		)
	} else if isStringType(fieldType) {
		// String: check empty
		grp.If(jen.Add(fieldAccess).Op("==").Lit("")).Block(
			jen.Id(mapId).Index(jen.Lit(fieldName)).Op("=").Lit("(empty)"),
		).Else().Block(
			jen.Id(mapId).Index(jen.Lit(fieldName)).Op("=").Lit("(sensitive)"),
		)
	} else {
		// Other types: just mark as sensitive
		grp.Id(mapId).Index(jen.Lit(fieldName)).Op("=").Lit("(sensitive)")
	}
}

// generateDebugCodeForStruct generates code for same-package struct fields (calls DebugMap)
func generateDebugCodeForStruct(grp *jen.Group, receiverId, fieldName, mapId string) {
	fieldAccess := jen.Id(receiverId).Dot(fieldName)

	// Call the DebugMap() method on the nested struct
	grp.Id(mapId).Index(jen.Lit(fieldName)).Op("=").Add(fieldAccess).Dot("DebugMap").Call()
}

// generateDebugCodeForStructFormat generates code for struct fields with inline flattening
func generateDebugCodeForStructFormat(grp *jen.Group, receiverId, fieldName, mapId string) {
	fieldAccess := jen.Id(receiverId).Dot(fieldName)

	// Call FlatDebugMap() on the nested struct and merge keys with dot notation
	nestedMapVar := "nested" + toTitle(fieldName)
	grp.Id(nestedMapVar).Op(":=").Add(fieldAccess).Dot("FlatDebugMap").Call()
	grp.For(jen.List(jen.Id("k"), jen.Id("v")).Op(":=").Range().Id(nestedMapVar)).Block(
		jen.Id(mapId).Index(jen.Lit(fieldName).Op("+").Lit(".").Op("+").Id("k")).Op("=").Id("v"),
	)
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

// toTitle capitalizes the first letter of a string (replaces deprecated strings.Title)
func toTitle(s string) string {
	if len(s) == 0 {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// formatFunctionName returns properly cased function name based on visibility
func formatFunctionName(prefix, fieldName, structPrefix string, makePublic bool) string {
	name := fmt.Sprintf("%s%s%s", prefix, structPrefix, toTitle(fieldName))
	if !makePublic {
		name = unexport(name)
	}
	return name
}
