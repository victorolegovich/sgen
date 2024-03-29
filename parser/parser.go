package parser

import (
	"errors"
	c "github.com/victorolegovich/sgen/collection"
	fm "github.com/victorolegovich/sgen/file_manager"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"path/filepath"
	"strings"
)

func Parse(dir string, collection *c.Collection) error {
	var (
		errText string
		errs    []error
	)

	files, err := ioutil.ReadDir(dir)

	if err != nil {
		return err
	}

	for _, file := range files {
		if !file.IsDir() {
			if strings.Split(file.Name(), ".")[1] == "go" {
				if err = parse(filepath.Join(dir, file.Name()), collection); err != nil {
					errs = append(errs, err)
				}
			}
		}
	}

	for _, e := range errs {
		errText += e.Error() + "\n"
	}

	if errText != "" {
		return errors.New(errText)
	}

	return nil
}

func parse(filename string, collection *c.Collection) error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		return err
	}

	inspecting(file, collection)

	//_ = ast.Print(fset, file)

	for i, s := range collection.Structs {
		if err := completeFile(filename, i, s, collection); err != nil {
			return err
		}
	}

	return nil
}

func inspecting(file *ast.File, collection *c.Collection) {
	var parents = map[string][]string{}

	ast.Inspect(file, func(node ast.Node) bool {
		var (
			Struct     c.Struct
			RootSchema c.RootSchema
			isParent   bool
		)

		switch n := node.(type) {
		case *ast.File:
			collection.DataPackage = n.Name.Name
		case *ast.TypeSpec:

			switch s := n.Type.(type) {

			case *ast.StructType:
				Struct.Name = n.Name.Name
				Struct.Fields, RootSchema.Childes, Struct.Complicated, isParent = fields(s)

				if isParent {
					for _, child := range RootSchema.Childes {
						parents[child.StructName] = append(parents[child.StructName], n.Name.Name)
					}
				}

				Struct.RootSchema = RootSchema

				collection.Structs = append(collection.Structs, Struct)
			}
		}

		return true
	})

	for i, s := range collection.Structs {
		for child, parentList := range parents {
			if s.Name == child {
				collection.Structs[i].Parents = parentList
			}
		}
	}
}

func fields(s *ast.StructType) (Fields []c.Field, Childes []c.RootObject, Complicated map[string]c.Complicated, parent bool) {
	var Field = c.Field{}
	var Child = c.RootObject{}

	Complicated = map[string]c.Complicated{}

	for _, field := range s.Fields.List {
		if len(field.Names) == 0 {
			switch ftype := field.Type.(type) {

			case *ast.Ident:
				Child.StructName, Child.Type, Child.Name = ftype.Name, ftype.Name, ftype.Name
				Childes = append(Childes, Child)
				Field.Name, Field.Type = ftype.Name, ftype.Name
				parent = true
				Fields = append(Fields, Field)
			case *ast.StarExpr:
				switch x := ftype.X.(type) {
				case *ast.Ident:
					Child.StructName, Child.Type, Child.Name = x.Name, "*"+x.Name, x.Name
					Childes = append(Childes, Child)
					Field.Name, Field.Type = x.Name, x.Name
					parent = true
					Fields = append(Fields, Field)
				}
			}
		}

		for _, ident := range field.Names {
			Field.Name = ident.Name

			comp, child := fieldType(field, &Field)

			if comp != c.Empty {
				Complicated[Field.Name] = comp
			}
			if child.StructName != "" {
				child.Field = Field
				parent = true
				Childes = append(Childes, child)
			}

			Fields = append(Fields, Field)
		}

	}

	return Fields, Childes, Complicated, parent
}

func fieldType(field *ast.Field, Field *c.Field) (complicated c.Complicated, child c.RootObject) {
	complicated = c.Empty

	switch ftype := field.Type.(type) {

	//Простой тип
	case *ast.Ident:
		Field.Type = ftype.Name

		//Усложнился вложенной структурой
		if ftype.Obj != nil {
			switch decl := ftype.Obj.Decl.(type) {

			case *ast.TypeSpec:
				switch decl.Type.(type) {
				case *ast.StructType:
					println(decl.Name)
					Field.Type = decl.Name.Name
					child.StructName = decl.Name.Name
				}
			}
		}

	case *ast.InterfaceType:
		if len(ftype.Methods.List) == 0 {
			Field.Type = "interface{}"
		} else {
			complicated = c.ComplicatedInterface
		}

	//Мапа
	case *ast.MapType:

		switch key := ftype.Key.(type) {

		case *ast.Ident:
			Field.Type = "map[" + key.Name + "]"
			switch typespec := key.Obj.Decl.(type) {
			case *ast.TypeSpec:
				switch typespec.Type.(type) {
				case *ast.StructType:
					complicated = c.ComplicatedMap
				}
			}
		}

		switch value := ftype.Value.(type) {

		case *ast.Ident:
			Field.Type += value.Name

			switch decl := value.Obj.Decl.(type) {
			case *ast.TypeSpec:
				switch decl.Type.(type) {
				case *ast.StructType:
					child.Type = value.Name
				}
			}

		//Это усложнённые типы, такое мы обрабатывать не будем.
		case *ast.MapType:
			complicated = c.ComplicatedMap

		case *ast.ArrayType:
			complicated = c.ComplicatedMap
		}

	//Массив
	case *ast.ArrayType:
		var arrayLen string

		switch arrlen := ftype.Len.(type) {
		case *ast.BasicLit:
			arrayLen = arrlen.Value
		}

		switch arrname := ftype.Elt.(type) {

		case *ast.Ident:

			if arrayLen != "" {
				Field.Type = "[" + arrayLen + "]" + arrname.Name
			} else {
				Field.Type = "[]" + arrname.Name
			}
			if arrname.Obj != nil {
				switch decl := arrname.Obj.Decl.(type) {
				case *ast.TypeSpec:
					switch decl.Type.(type) {
					case *ast.StructType:
						child.StructName = arrname.Name
					}
				}
			}

		//Это усложнённые типы, такое мы обрабатывать не будем.
		case *ast.MapType:
			complicated = c.ComplicatedArray

		case *ast.ArrayType:
			complicated = c.ComplicatedArray
		}

	//Скорее всего, это импортируемый тип.
	case *ast.SelectorExpr:
		switch x := ftype.X.(type) {

		case *ast.Ident:
			Field.Type = x.Name + "." + ftype.Sel.Name
		}

	case *ast.StarExpr:
		switch x := ftype.X.(type) {

		case *ast.SelectorExpr:
			switch x2 := x.X.(type) {
			case *ast.Ident:
				Field.Type = "*" + x2.Name + "." + x.Sel.Name
			}
		}
	}

	return complicated, child
}

func completeFile(filename string, i int, s c.Struct, collection *c.Collection) error {
	var needle, exp string

	exp = "(type " + s.Name + " struct[ {])"

	pos := 0

	for _, field := range checkFields(s) {
		if field.Name != "ID" {
			pos++
		}
		needle += "\t" + field.Name + " " + field.Type + "\n"

		addField(collection, field, i, pos)
	}

	if needle != "" {
		err := fm.AddToFile(filename, exp, needle, fm.Decl)
		if err != nil {
			return err
		}
	}

	return nil
}

func checkFields(s c.Struct) (fields []c.Field) {
	idField := c.Field{Name: "ID", Type: "int"}
	if !hasField(s.Fields, idField) {
		fields = append(fields, idField)
	}

	for _, parent := range s.Parents {
		parentId := c.Field{Name: parent + "ID", Type: "int"}

		if !hasField(s.Fields, parentId) {
			fields = append(fields, parentId)
		}
	}

	return fields
}

func hasField(fields []c.Field, field c.Field) bool {
	for _, f := range fields {
		if f.Name == field.Name && f.Type == field.Type {
			return true
		}
	}
	return false
}

func addField(collection *c.Collection, field c.Field, index, pos int) {
	f := collection.Structs[index].Fields

	fields := f[:pos]
	fields = append(fields, field)
	fields = append(fields, f[pos:]...)

	collection.Structs[index].Fields = fields
}
