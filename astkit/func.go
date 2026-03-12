package astkit

import "go/ast"

// Func wraps a FuncDecl with convenience methods.
type Func struct {
	*ast.FuncDecl
}

// WrapFunc creates a Func wrapper.
func WrapFunc(fn *ast.FuncDecl) *Func {
	if fn == nil {
		return nil
	}
	return &Func{fn}
}

// HasReceiver returns true if this is a method.
func (f *Func) HasReceiver() bool {
	if f == nil || f.FuncDecl == nil {
		return false
	}
	return f.Recv != nil && len(f.Recv.List) > 0
}

// Receiver returns the receiver field, or nil.
func (f *Func) Receiver() *ast.Field {
	if !f.HasReceiver() {
		return nil
	}
	return f.Recv.List[0]
}

// ReceiverAsField returns the receiver as a SimpleField with exported name.
func (f *Func) ReceiverAsField() *SimpleField {
	recv := f.Receiver()
	if recv == nil {
		return nil
	}
	name := "Receiver"
	if len(recv.Names) > 0 && recv.Names[0].Name != "" {
		name = Export(recv.Names[0].Name)
	}
	return &SimpleField{Name: name, Type: recv.Type}
}

// RemoveReceiver removes the receiver, converting method to function.
func (f *Func) RemoveReceiver() {
	if f != nil && f.FuncDecl != nil {
		f.Recv = nil
	}
}

// Params returns the parameters as SimpleFields.
func (f *Func) Params() []SimpleField {
	if f == nil || f.FuncDecl == nil {
		return nil
	}
	return FieldsFromList(f.Type.Params)
}

// Results returns the results as SimpleFields.
func (f *Func) Results() []SimpleField {
	if f == nil || f.FuncDecl == nil {
		return nil
	}
	return FieldsFromList(f.Type.Results)
}

// ParamCount returns the number of parameters.
func (f *Func) ParamCount() int {
	if f == nil || f.FuncDecl == nil {
		return 0
	}
	return FieldCount(f.Type.Params)
}

// ResultCount returns the number of results.
func (f *Func) ResultCount() int {
	if f == nil || f.FuncDecl == nil {
		return 0
	}
	return FieldCount(f.Type.Results)
}

// HasContextParam returns true if first param is context.Context.
func (f *Func) HasContextParam() bool {
	if f == nil || f.FuncDecl == nil {
		return false
	}
	if f.Type.Params == nil || len(f.Type.Params.List) == 0 {
		return false
	}
	return IsContextType(f.Type.Params.List[0].Type)
}

// SetParams replaces the parameter list.
func (f *Func) SetParams(fl *ast.FieldList) {
	if f != nil && f.FuncDecl != nil {
		f.Type.Params = fl
	}
}

// SetResults replaces the result list.
func (f *Func) SetResults(fl *ast.FieldList) {
	if f != nil && f.FuncDecl != nil {
		f.Type.Results = fl
	}
}

// PrependParam adds a parameter at the beginning.
func (f *Func) PrependParam(field *ast.Field) {
	if f == nil || f.FuncDecl == nil {
		return
	}
	if f.Type.Params == nil {
		f.Type.Params = &ast.FieldList{}
	}
	f.Type.Params.List = append([]*ast.Field{field}, f.Type.Params.List...)
}

// PrependStmt adds a statement at the beginning of the body.
func (f *Func) PrependStmt(stmt ast.Stmt) {
	if f == nil || f.FuncDecl == nil {
		return
	}
	if f.Body == nil {
		f.Body = &ast.BlockStmt{}
	}
	f.Body.List = append([]ast.Stmt{stmt}, f.Body.List...)
}

// PrependStmts adds statements at the beginning of the body.
func (f *Func) PrependStmts(stmts ...ast.Stmt) {
	if f == nil || f.FuncDecl == nil {
		return
	}
	if f.Body == nil {
		f.Body = &ast.BlockStmt{}
	}
	f.Body.List = append(stmts, f.Body.List...)
}

// IsSingleStructParam returns true if there's exactly one struct-typed param.
func (f *Func) IsSingleStructParam() bool {
	if f == nil || f.FuncDecl == nil {
		return false
	}
	return IsSingleStructField(f.Type.Params)
}

// IsSingleStructResult returns true if non-error results are a single struct.
func (f *Func) IsSingleStructResult() bool {
	if f == nil || f.FuncDecl == nil {
		return false
	}
	results := f.Results()
	nonError, _ := SeparateError(results)
	if len(nonError) != 1 {
		return false
	}
	return IsStructType(nonError[0].Type)
}

// AllInputFields returns receiver (if any) + params as SimpleFields.
func (f *Func) AllInputFields() []SimpleField {
	if f == nil || f.FuncDecl == nil {
		return nil
	}
	var fields []SimpleField
	if recv := f.ReceiverAsField(); recv != nil {
		fields = append(fields, *recv)
	}
	fields = append(fields, f.Params()...)
	return fields
}
