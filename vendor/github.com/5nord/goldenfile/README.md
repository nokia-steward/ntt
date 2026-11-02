# goldenfile
Go test helper to verify output against golden files

**Example**

```
===
simple
===
var integer a;
---
(Root
  (VarDecl
    (Reference)
    (Declarators
      (Declarator
        (Name "a")))))

===
with assignment
===
var integer a := 5;
---
(Root
  (VarDecl
    (Reference)
    (Declarators
      (Declarator
        (Name "a")
        (Literal "5")))))
```

**Testing**

```go
tests, err := goldenfile.Parse(fh)
if err != nil {
	t.Fatalf("goldenfile.Parse(%s): %s", file, err.Error())
}

if *goldenfile.Update {
	updateTests(t, tests)
}

for _, test := range tests {
	t.Run(test.Name, func (t *testing.T) {
		if myFunc(test.Text) != test.Want {
			log.Errorf("myFunc() failed")
		}
	})
}


```
