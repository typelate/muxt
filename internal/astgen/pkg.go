package astgen

//type TypeFormatter struct {
//	outputPkgPath string
//	imports       map[string]string
//}
//
//func NewTypeFormatter(outputPkgPath string, pkg ...string) *TypeFormatter {
//	m := make(map[string]string)
//	for _, imp := range imports {
//		m[imp] =
//	}
//	return &TypeFormatter{
//		outputPkgPath: outputPkgPath,
//		imports:       ,
//	}
//}
//
//func (tf *TypeFormatter) Qualifier(pkg *types.Package) string {
//	if pkg == nil {
//		return ""
//	}
//	if pkg.Path() == tf.outputPkgPath {
//		return ""
//	}
//	tf.imports[pkg.Path()] = pkg.Name()
//	return pkg.Name()
//}
