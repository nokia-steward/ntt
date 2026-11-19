# Current Conformance Misses

Generated from `etsi-report7.json` using the current workspace runner.

- Summary: `ran 5177 files: 5085 matched, 13 skipped (98.47% match rate)`
- Misses: `92`

## Misses By Outcome

- `reject->pass`: `39`
- `reject->none`: `23`
- `->`: `13`
- `pass->fail`: `6`
- `pass->error`: `5`
- `pass->none`: `2`
- `pass->parse-error`: `2`
- `pass->semantic-error`: `2`

## Misses By Chapter

- `core_language/06_types_and_values`: `35` (`->`=6, `pass->error`=2, `pass->fail`=4, `pass->none`=1, `pass->semantic-error`=1, `reject->none`=12, `reject->pass`=9)
- `core_language/05_basic_language_elements`: `14` (`->`=2, `reject->none`=1, `reject->pass`=11)
- `core_language/22_communication_operations`: `11` (`reject->none`=1, `reject->pass`=10)
- `core_language/15_templates`: `9` (`->`=2, `pass->fail`=1, `reject->none`=3, `reject->pass`=3)
- `core_language/16_functions_altsteps_testcases`: `5` (`->`=1, `pass->error`=1, `pass->semantic-error`=1, `reject->none`=1, `reject->pass`=1)
- `core_language/20_statement_and_operations_for_alt`: `3` (`pass->error`=1, `pass->fail`=1, `reject->pass`=1)
- `core_language/B_matching_incoming_values`: `3` (`reject->none`=2, `reject->pass`=1)
- `oo/501_classes_and_objects`: `3` (`pass->parse-error`=2, `reject->pass`=1)
- `core_language/07_expressions`: `2` (`reject->pass`=2)
- `core_language/09_test_configurations`: `2` (`pass->error`=1, `reject->none`=1)
- `core_language/27_specifying_attributes`: `2` (`->`=1, `reject->none`=1)
- `core_language/08_modules`: `1` (`->`=1)
- `core_language/C_predefined_functions`: `1` (`pass->none`=1)
- `core_language/D_preprocessing_macros`: `1` (`reject->none`=1)

## Top Chapter Details

### `core_language/06_types_and_values`

- `core_language/06_types_and_values/0601_basic_types_and_values/060102_subtyping_of_basic_types/06010202_lists_of_types/NegSem_06010202_ListOfTypes_005.ttcn`: `reject -> pass`
- `core_language/06_types_and_values/0601_basic_types_and_values/060102_subtyping_of_basic_types/06010205_pattern_subtyping_of_character_string_types/NegSyn_06010205_StringPattern_002.ttcn`: `reject -> none`
- `core_language/06_types_and_values/0602_structured_types_and_values/060201_record_type_and_values/06020101_referencing_fields_of_record_type/NegSem_06020101_ReferencingRecordFields_001.ttcn`: `reject -> none`
- `core_language/06_types_and_values/0602_structured_types_and_values/060201_record_type_and_values/06020104_embedding_fields/Sem_06020104_embedding_fields_002.ttcn`: `pass -> fail`
- `core_language/06_types_and_values/0602_structured_types_and_values/060201_record_type_and_values/06020104_embedding_fields/Sem_06020104_embedding_fields_003.ttcn`: `pass -> fail` - field c not promoted
- `core_language/06_types_and_values/0602_structured_types_and_values/060201_record_type_and_values/06020104_embedding_fields/Sem_06020104_embedding_fields_004.ttcn`: `pass -> fail`
- `core_language/06_types_and_values/0602_structured_types_and_values/060201_record_type_and_values/060201_toplevel/NegSem_060201_RecordTypeValues_001.ttcn`: `reject -> none`
- `core_language/06_types_and_values/0602_structured_types_and_values/060202_set_type_and_values/NegSem_060202_SetTypeValues_001.ttcn`: `reject -> none`
- `core_language/06_types_and_values/0602_structured_types_and_values/060202_set_type_and_values/NegSem_060202_SetTypeValues_002.ttcn`: `reject -> none`
- `core_language/06_types_and_values/0602_structured_types_and_values/060203_records_and_sets_of_single_types/NegSem_060203_records_and_sets_of_single_types_001.ttcn`: `reject -> none`
- `core_language/06_types_and_values/0602_structured_types_and_values/060203_records_and_sets_of_single_types/NegSem_060203_records_and_sets_of_single_types_002.ttcn`: `reject -> none`
- `core_language/06_types_and_values/0602_structured_types_and_values/060205_unions/06020501_referencing_fields_of_union_type/NegSem_06020501_referencing_fields_of_union_type_005.ttcn`: `reject -> pass`
- `core_language/06_types_and_values/0602_structured_types_and_values/060205_unions/06020504_embedding_fields/Sem_06020504_embedding_fields_001.ttcn`: `pass -> fail` - a.R is not chosen
- `core_language/06_types_and_values/0602_structured_types_and_values/060207_arrays/NegSyn_060207_arrays_004.ttcn`: ` -> `
- `core_language/06_types_and_values/0602_structured_types_and_values/060212_addressing_entities_inside_sut/NegSem_060212_AddressingEntitiesInsideSut_002.ttcn`: `reject -> pass`
- `core_language/06_types_and_values/0602_structured_types_and_values/060212_addressing_entities_inside_sut/NegSem_060212_AddressingEntitiesInsideSut_003.ttcn`: `reject -> pass`
- `core_language/06_types_and_values/0602_structured_types_and_values/060212_addressing_entities_inside_sut/NegSem_060212_AddressingEntitiesInsideSut_004.ttcn`: `reject -> pass`
- `core_language/06_types_and_values/0602_structured_types_and_values/060213_subtyping_of_structured_types/06021302_list_subtyping/NegSem_06021302_ListSubtyping_001.ttcn`: ` -> `
- `core_language/06_types_and_values/0602_structured_types_and_values/060213_subtyping_of_structured_types/06021302_list_subtyping/NegSem_06021302_ListSubtyping_002.ttcn`: ` -> `
- `core_language/06_types_and_values/0602_structured_types_and_values/060213_subtyping_of_structured_types/06021302_list_subtyping/NegSem_06021302_ListSubtyping_003.ttcn`: ` -> `
- ... `15` more in `current-misses.json`

### `core_language/05_basic_language_elements`

- `core_language/05_basic_language_elements/0504_parametrization/050401_formal_parameters/05040101_parameters_of_kind_value/NegSem_05040101_parameters_of_kind_value_010.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050401_formal_parameters/05040102_parameters_of_kind_template/NegSem_05040102_parameters_of_kind_template_010.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050401_formal_parameters/05040102_parameters_of_kind_template/NegSem_05040102_parameters_of_kind_template_020.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050401_formal_parameters/05040102_parameters_of_kind_template/NegSem_05040102_parameters_of_kind_template_021.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050401_formal_parameters/05040104_parameters_of_kind_port/NegSem_05040104_parameters_of_kind_port_004.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050402_actual_parameters/NegSem_050402_actual_parameters_221.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050402_actual_parameters/NegSem_050402_actual_parameters_223.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050402_actual_parameters/NegSem_050402_actual_parameters_224.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050402_actual_parameters/NegSem_050402_actual_parameters_225.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050402_actual_parameters/NegSem_050402_actual_parameters_229.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050402_actual_parameters/NegSem_050402_actual_parameters_230.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/0504_toplevel/NegSyn_0504_forbidden_parametrization_001.ttcn`: `reject -> none`
- `core_language/05_basic_language_elements/0505_cyclic_definitions/NegSem_0505_cyclic_definitions_001.ttcn`: ` -> `
- `core_language/05_basic_language_elements/05_toplevel/NegSyn_05_TopLevel_001.ttcn`: ` -> `

### `core_language/22_communication_operations`

- `core_language/22_communication_operations/2202_message_based_communication/220201_send_operation/NegSem_220201_SendOperation_011.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2202_message_based_communication/220202_receive_operation/NegSem_220202_ReceiveOperation_023.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2202_message_based_communication/220203_trigger_operation/NegSem_220203_TriggerOperation_023.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2203_procedure_based_communication/220301_call_operation/NegSem_220301_CallOperation_020.ttcn`: `reject -> none`
- `core_language/22_communication_operations/2203_procedure_based_communication/220302_getcall_operation/NegSem_220302_getcall_operation_017.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2203_procedure_based_communication/220303_reply_operation/NegSem_220303_ReplyOperation_010.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2203_procedure_based_communication/220304_getreply_operation/NegSem_220304_getreply_operation_014.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2203_procedure_based_communication/220304_getreply_operation/NegSem_220304_getreply_operation_020.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2203_procedure_based_communication/220305_raise_operation/NegSem_220305_raise_operation_009.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2203_procedure_based_communication/220306_catch_operation/NegSem_220306_catch_operation_014.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2204_the_check_operation/NegSem_2204_the_check_operation_005.ttcn`: `reject -> pass`
