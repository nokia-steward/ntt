# Current Conformance Misses

Generated from `off.json` using the current workspace runner.

- Summary: `ran 5164 files: 4953 matched, 21 skipped (96.31% match rate)`
- Misses: `211`

## Misses By Outcome

- `reject->pass`: `67`
- `pass->parse-error`: `35`
- `reject->none`: `27`
- `pass->error`: `25`
- `->`: `21`
- `pass->fail`: `18`
- `error->parse-error`: `8`
- `pass->semantic-error`: `7`
- `pass->none`: `3`

## Misses By Chapter

- `core_language/06_types_and_values`: `62` (`->`=8, `pass->error`=6, `pass->fail`=2, `pass->none`=1, `pass->parse-error`=12, `pass->semantic-error`=2, `reject->none`=13, `reject->pass`=18)
- `core_language/22_communication_operations`: `36` (`pass->error`=12, `pass->fail`=4, `reject->none`=1, `reject->pass`=19)
- `core_language/05_basic_language_elements`: `26` (`->`=2, `pass->parse-error`=11, `reject->none`=1, `reject->pass`=12)
- `core_language/15_templates`: `22` (`->`=2, `pass->error`=2, `pass->fail`=7, `reject->none`=3, `reject->pass`=8)
- `core_language/16_functions_altsteps_testcases`: `20` (`->`=1, `error->parse-error`=8, `pass->none`=1, `pass->parse-error`=5, `pass->semantic-error`=1, `reject->none`=1, `reject->pass`=3)
- `core_language/B_matching_incoming_values`: `10` (`pass->error`=2, `pass->fail`=5, `reject->none`=2, `reject->pass`=1)
- `core_language/09_test_configurations`: `8` (`pass->error`=1, `reject->none`=4, `reject->pass`=3)
- `core_language/07_expressions`: `6` (`pass->semantic-error`=4, `reject->pass`=2)
- `core_language/19_basic_program_statements`: `6` (`pass->error`=2, `pass->parse-error`=4)
- `core_language/26_module_control`: `3` (`->`=3)
- `oo/501_classes_and_objects`: `3` (`pass->parse-error`=2, `reject->pass`=1)
- `core_language/08_modules`: `2` (`->`=2)
- `core_language/11_variables`: `2` (`->`=2)
- `core_language/27_specifying_attributes`: `2` (`->`=1, `reject->none`=1)
- `core_language/10_constants`: `1` (`pass->parse-error`=1)
- `core_language/C_predefined_functions`: `1` (`pass->none`=1)
- `core_language/D_preprocessing_macros`: `1` (`reject->none`=1)

## Top Chapter Details

### `core_language/06_types_and_values`

- `core_language/06_types_and_values/0601_basic_types_and_values/060100_simple_basic_types_and_values/NegSyn_060100_SimpleBasicTypes_005.ttcn`: ` -> `
- `core_language/06_types_and_values/0601_basic_types_and_values/060100_simple_basic_types_and_values/NegSyn_060100_SimpleBasicTypes_006.ttcn`: ` -> `
- `core_language/06_types_and_values/0601_basic_types_and_values/060102_subtyping_of_basic_types/06010202_lists_of_types/NegSem_06010202_ListOfTypes_005.ttcn`: `reject -> pass`
- `core_language/06_types_and_values/0601_basic_types_and_values/060102_subtyping_of_basic_types/06010205_pattern_subtyping_of_character_string_types/NegSyn_06010205_StringPattern_002.ttcn`: `reject -> none`
- `core_language/06_types_and_values/0602_structured_types_and_values/060201_record_type_and_values/06020101_referencing_fields_of_record_type/NegSem_06020101_ReferencingRecordFields_001.ttcn`: `reject -> none`
- `core_language/06_types_and_values/0602_structured_types_and_values/060201_record_type_and_values/06020104_embedding_fields/Sem_06020104_embedding_fields_001.ttcn`: `pass -> parse-error` - 4 errors occurred:
	* {testdata/ttcn3-conformance-tests/ATS/core_language/06_types_and_values/0602_structured_types_and_values/060201_record_type_and_values/06020104_embedding_fields/Sem_06020104_embe...
- `core_language/06_types_and_values/0602_structured_types_and_values/060201_record_type_and_values/06020104_embedding_fields/Sem_06020104_embedding_fields_002.ttcn`: `pass -> parse-error` - 4 errors occurred:
	* {testdata/ttcn3-conformance-tests/ATS/core_language/06_types_and_values/0602_structured_types_and_values/060201_record_type_and_values/06020104_embedding_fields/Sem_06020104_embe...
- `core_language/06_types_and_values/0602_structured_types_and_values/060201_record_type_and_values/06020104_embedding_fields/Sem_06020104_embedding_fields_003.ttcn`: `pass -> parse-error` - 4 errors occurred:
	* {testdata/ttcn3-conformance-tests/ATS/core_language/06_types_and_values/0602_structured_types_and_values/060201_record_type_and_values/06020104_embedding_fields/Sem_06020104_embe...
- `core_language/06_types_and_values/0602_structured_types_and_values/060201_record_type_and_values/06020104_embedding_fields/Sem_06020104_embedding_fields_004.ttcn`: `pass -> parse-error` - 4 errors occurred:
	* {testdata/ttcn3-conformance-tests/ATS/core_language/06_types_and_values/0602_structured_types_and_values/060201_record_type_and_values/06020104_embedding_fields/Sem_06020104_embe...
- `core_language/06_types_and_values/0602_structured_types_and_values/060201_record_type_and_values/060201_toplevel/NegSem_060201_RecordTypeValues_001.ttcn`: `reject -> none`
- `core_language/06_types_and_values/0602_structured_types_and_values/060202_set_type_and_values/06020204_embedding_fields/Sem_06020204_embedding_fields_001.ttcn`: `pass -> parse-error` - 2 errors occurred:
	* {testdata/ttcn3-conformance-tests/ATS/core_language/06_types_and_values/0602_structured_types_and_values/060202_set_type_and_values/06020204_embedding_fields/Sem_06020204_embeddi...
- `core_language/06_types_and_values/0602_structured_types_and_values/060202_set_type_and_values/NegSem_060202_SetTypeValues_001.ttcn`: `reject -> none`
- `core_language/06_types_and_values/0602_structured_types_and_values/060202_set_type_and_values/NegSem_060202_SetTypeValues_002.ttcn`: `reject -> none`
- `core_language/06_types_and_values/0602_structured_types_and_values/060203_records_and_sets_of_single_types/NegSem_060203_records_and_sets_of_single_types_001.ttcn`: `reject -> none`
- `core_language/06_types_and_values/0602_structured_types_and_values/060203_records_and_sets_of_single_types/NegSem_060203_records_and_sets_of_single_types_002.ttcn`: `reject -> none`
- `core_language/06_types_and_values/0602_structured_types_and_values/060205_unions/06020501_referencing_fields_of_union_type/NegSem_06020501_referencing_fields_of_union_type_005.ttcn`: `reject -> pass`
- `core_language/06_types_and_values/0602_structured_types_and_values/060205_unions/06020502_option_and_union/NegSyn_06020502_option_and_union_001.ttcn`: ` -> `
- `core_language/06_types_and_values/0602_structured_types_and_values/060205_unions/06020504_embedding_fields/Sem_06020504_embedding_fields_001.ttcn`: `pass -> parse-error` - 6 errors occurred:
	* {testdata/ttcn3-conformance-tests/ATS/core_language/06_types_and_values/0602_structured_types_and_values/060205_unions/06020504_embedding_fields/Sem_06020504_embedding_fields_001...
- `core_language/06_types_and_values/0602_structured_types_and_values/060206_anytype/NegSem_060206_anytype_001.ttcn`: `reject -> pass`
- `core_language/06_types_and_values/0602_structured_types_and_values/060207_arrays/NegSem_060207_arrays_026.ttcn`: `reject -> pass`
- ... `42` more in `current-misses.json`

### `core_language/22_communication_operations`

- `core_language/22_communication_operations/2202_message_based_communication/220201_send_operation/NegSem_220201_SendOperation_005.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2202_message_based_communication/220201_send_operation/NegSem_220201_SendOperation_011.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2202_message_based_communication/220202_receive_operation/NegSem_220202_ReceiveOperation_006.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2202_message_based_communication/220202_receive_operation/NegSem_220202_ReceiveOperation_023.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2202_message_based_communication/220203_trigger_operation/NegSem_220203_TriggerOperation_006.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2202_message_based_communication/220203_trigger_operation/NegSem_220203_TriggerOperation_023.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2203_procedure_based_communication/220301_call_operation/NegSem_220301_CallOperation_012.ttcn`: `reject -> pass` - Both replies received
- `core_language/22_communication_operations/2203_procedure_based_communication/220301_call_operation/NegSem_220301_CallOperation_020.ttcn`: `reject -> none`
- `core_language/22_communication_operations/2203_procedure_based_communication/220302_getcall_operation/NegSem_220302_getcall_operation_012.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2203_procedure_based_communication/220302_getcall_operation/NegSem_220302_getcall_operation_017.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2203_procedure_based_communication/220303_reply_operation/NegSem_220303_ReplyOperation_008.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2203_procedure_based_communication/220303_reply_operation/NegSem_220303_ReplyOperation_010.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2203_procedure_based_communication/220304_getreply_operation/NegSem_220304_getreply_operation_009.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2203_procedure_based_communication/220304_getreply_operation/NegSem_220304_getreply_operation_014.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2203_procedure_based_communication/220304_getreply_operation/NegSem_220304_getreply_operation_016.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2203_procedure_based_communication/220304_getreply_operation/NegSem_220304_getreply_operation_020.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2203_procedure_based_communication/220305_raise_operation/NegSem_220305_raise_operation_004.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2203_procedure_based_communication/220305_raise_operation/NegSem_220305_raise_operation_009.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2203_procedure_based_communication/220306_catch_operation/NegSem_220306_catch_operation_014.ttcn`: `reject -> pass`
- `core_language/22_communication_operations/2204_the_check_operation/NegSem_2204_the_check_operation_005.ttcn`: `reject -> pass`
- ... `16` more in `current-misses.json`

### `core_language/05_basic_language_elements`

- `core_language/05_basic_language_elements/0504_parametrization/050401_formal_parameters/05040101_parameters_of_kind_value/NegSem_05040101_parameters_of_kind_value_010.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050401_formal_parameters/05040102_parameters_of_kind_template/NegSem_05040102_parameters_of_kind_template_010.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050401_formal_parameters/05040102_parameters_of_kind_template/NegSem_05040102_parameters_of_kind_template_020.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050401_formal_parameters/05040102_parameters_of_kind_template/NegSem_05040102_parameters_of_kind_template_021.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050401_formal_parameters/05040103_parameters_of_kind_timer/NegSyn_05040103_parameters_of_kind_timer_002.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050401_formal_parameters/05040104_parameters_of_kind_port/NegSem_05040104_parameters_of_kind_port_004.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050402_actual_parameters/NegSem_050402_actual_parameters_221.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050402_actual_parameters/NegSem_050402_actual_parameters_223.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050402_actual_parameters/NegSem_050402_actual_parameters_224.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050402_actual_parameters/NegSem_050402_actual_parameters_225.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050402_actual_parameters/NegSem_050402_actual_parameters_229.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050402_actual_parameters/NegSem_050402_actual_parameters_230.ttcn`: `reject -> pass`
- `core_language/05_basic_language_elements/0504_parametrization/050403_variadic_parameters/Sem_050403_variadic_parameters_001.ttcn`: `pass -> parse-error` - 3 errors occurred:
	* {testdata/ttcn3-conformance-tests/ATS/core_language/05_basic_language_elements/0504_parametrization/050403_variadic_parameters/Sem_050403_variadic_parameters_001.ttcn 8:37 8:40}:...
- `core_language/05_basic_language_elements/0504_parametrization/050403_variadic_parameters/Sem_050403_variadic_parameters_002.ttcn`: `pass -> parse-error` - 3 errors occurred:
	* {testdata/ttcn3-conformance-tests/ATS/core_language/05_basic_language_elements/0504_parametrization/050403_variadic_parameters/Sem_050403_variadic_parameters_002.ttcn 6:24 6:27}:...
- `core_language/05_basic_language_elements/0504_parametrization/050403_variadic_parameters/Sem_050403_variadic_parameters_003.ttcn`: `pass -> parse-error` - 3 errors occurred:
	* {testdata/ttcn3-conformance-tests/ATS/core_language/05_basic_language_elements/0504_parametrization/050403_variadic_parameters/Sem_050403_variadic_parameters_003.ttcn 6:29 6:32}:...
- `core_language/05_basic_language_elements/0504_parametrization/050403_variadic_parameters/Sem_050403_variadic_parameters_004.ttcn`: `pass -> parse-error` - 3 errors occurred:
	* {testdata/ttcn3-conformance-tests/ATS/core_language/05_basic_language_elements/0504_parametrization/050403_variadic_parameters/Sem_050403_variadic_parameters_004.ttcn 8:71 8:74}:...
- `core_language/05_basic_language_elements/0504_parametrization/050403_variadic_parameters/Sem_050403_variadic_parameters_005.ttcn`: `pass -> parse-error` - 3 errors occurred:
	* {testdata/ttcn3-conformance-tests/ATS/core_language/05_basic_language_elements/0504_parametrization/050403_variadic_parameters/Sem_050403_variadic_parameters_005.ttcn 6:24 6:27}:...
- `core_language/05_basic_language_elements/0504_parametrization/050403_variadic_parameters/Sem_050403_variadic_parameters_006.ttcn`: `pass -> parse-error` - 4 errors occurred:
	* {testdata/ttcn3-conformance-tests/ATS/core_language/05_basic_language_elements/0504_parametrization/050403_variadic_parameters/Sem_050403_variadic_parameters_006.ttcn 11:25 11:28...
- `core_language/05_basic_language_elements/0504_parametrization/050403_variadic_parameters/Sem_050403_variadic_parameters_007.ttcn`: `pass -> parse-error` - 25 errors occurred:
	* {testdata/ttcn3-conformance-tests/ATS/core_language/05_basic_language_elements/0504_parametrization/050403_variadic_parameters/Sem_050403_variadic_parameters_007.ttcn 17:29 17:3...
- `core_language/05_basic_language_elements/0504_parametrization/050403_variadic_parameters/Sem_050403_variadic_parameters_008.ttcn`: `pass -> parse-error` - 25 errors occurred:
	* {testdata/ttcn3-conformance-tests/ATS/core_language/05_basic_language_elements/0504_parametrization/050403_variadic_parameters/Sem_050403_variadic_parameters_008.ttcn 18:29 18:3...
- ... `6` more in `current-misses.json`

### `core_language/15_templates`

- `core_language/15_templates/1503_global_and_local_templates/NegSyn_1503_GlobalAndLocalTemplates_006.ttcn`: ` -> `
- `core_language/15_templates/1506_referencing_elements_of_templates_or_template_fields/150601_referencing_individual_string_elements/Sem_150601_ReferencingIndividualStringElements_001.ttcn`: `pass -> fail`
- `core_language/15_templates/1506_referencing_elements_of_templates_or_template_fields/150601_referencing_individual_string_elements/Sem_150601_ReferencingIndividualStringElements_002.ttcn`: `pass -> fail`
- `core_language/15_templates/1506_referencing_elements_of_templates_or_template_fields/150601_referencing_individual_string_elements/Sem_150601_ReferencingIndividualStringElements_003.ttcn`: `pass -> fail`
- `core_language/15_templates/1506_referencing_elements_of_templates_or_template_fields/150601_referencing_individual_string_elements/Sem_150601_ReferencingIndividualStringElements_005.ttcn`: `pass -> fail`
- `core_language/15_templates/1506_referencing_elements_of_templates_or_template_fields/150603_referencing_record_of_and_set_elements/NegSem_150603_ReferencingRecordOfAndSetElements_010.ttcn`: `reject -> pass`
- `core_language/15_templates/1506_referencing_elements_of_templates_or_template_fields/150603_referencing_record_of_and_set_elements/NegSem_150603_ReferencingRecordOfAndSetElements_011.ttcn`: `reject -> none`
- `core_language/15_templates/1506_referencing_elements_of_templates_or_template_fields/150603_referencing_record_of_and_set_elements/NegSem_150603_ReferencingRecordOfAndSetElements_012.ttcn`: `reject -> none`
- `core_language/15_templates/1506_referencing_elements_of_templates_or_template_fields/150603_referencing_record_of_and_set_elements/NegSem_150603_ReferencingRecordOfAndSetElements_013.ttcn`: `reject -> none`
- `core_language/15_templates/1506_referencing_elements_of_templates_or_template_fields/150603_referencing_record_of_and_set_elements/NegSem_150603_ReferencingRecordOfAndSetElements_015.ttcn`: `reject -> pass`
- `core_language/15_templates/1506_referencing_elements_of_templates_or_template_fields/150606_Referencing_map_elements/NegSem_150606_Referencing_map_elements_001.ttcn`: `reject -> pass`
- `core_language/15_templates/1506_referencing_elements_of_templates_or_template_fields/150606_Referencing_map_elements/NegSem_150606_Referencing_map_elements_003.ttcn`: `reject -> pass`
- `core_language/15_templates/1506_referencing_elements_of_templates_or_template_fields/150606_Referencing_map_elements/NegSem_150606_Referencing_map_elements_005.ttcn`: `reject -> pass`
- `core_language/15_templates/1506_referencing_elements_of_templates_or_template_fields/150606_Referencing_map_elements/NegSem_150606_Referencing_map_elements_006.ttcn`: `reject -> pass`
- `core_language/15_templates/1506_referencing_elements_of_templates_or_template_fields/150606_Referencing_map_elements/Sem_150606_Referencing_map_elements_001.ttcn`: `pass -> fail`
- `core_language/15_templates/1506_referencing_elements_of_templates_or_template_fields/150606_Referencing_map_elements/Sem_150606_Referencing_map_elements_003.ttcn`: `pass -> fail`
- `core_language/15_templates/1506_referencing_elements_of_templates_or_template_fields/150606_Referencing_map_elements/Sem_150606_Referencing_map_elements_004.ttcn`: `pass -> fail`
- `core_language/15_templates/1506_referencing_elements_of_templates_or_template_fields/150606_Referencing_map_elements/Sem_150606_Referencing_map_elements_005.ttcn`: `pass -> error` - Error: array index must be an integer, got string
- `core_language/15_templates/1506_referencing_elements_of_templates_or_template_fields/150606_Referencing_map_elements/Sem_150606_Referencing_map_elements_006.ttcn`: `pass -> error` - Error: array index must be an integer, got string
- `core_language/15_templates/1511_concatenating_templates_of_string_and_list_types/NegSem_1511_ConcatenatingTemplatesOfStringAndListTypes_004.ttcn`: `reject -> pass`
- ... `2` more in `current-misses.json`

### `core_language/16_functions_altsteps_testcases`

- `core_language/16_functions_altsteps_testcases/1601_functions/160102_predefined_functions/NegSem_160102_predefined_functions_009.ttcn`: `reject -> none`
- `core_language/16_functions_altsteps_testcases/1601_functions/160102_predefined_functions/NegSem_160102_predefined_functions_039.ttcn`: `reject -> pass`
- `core_language/16_functions_altsteps_testcases/1601_functions/160105_explicit_control_functions/NegSem_160105_explicit_control_functions_009.ttcn`: `reject -> pass`
- `core_language/16_functions_altsteps_testcases/1601_functions/160106_the_not_implemented_function/NegSem_160106_the_not_implemented_function_000.ttcn`: `error -> parse-error` - 1 error occurred:
	* {testdata/ttcn3-conformance-tests/ATS/core_language/16_functions_altsteps_testcases/1601_functions/160106_the_not_implemented_function/NegSem_160106_the_not_implemented_function_0...
- `core_language/16_functions_altsteps_testcases/1601_functions/160106_the_not_implemented_function/NegSem_160106_the_not_implemented_function_001.ttcn`: `error -> parse-error` - 1 error occurred:
	* {testdata/ttcn3-conformance-tests/ATS/core_language/16_functions_altsteps_testcases/1601_functions/160106_the_not_implemented_function/NegSem_160106_the_not_implemented_function_0...
- `core_language/16_functions_altsteps_testcases/1601_functions/160106_the_not_implemented_function/NegSem_160106_the_not_implemented_function_002.ttcn`: `error -> parse-error` - 1 error occurred:
	* {testdata/ttcn3-conformance-tests/ATS/core_language/16_functions_altsteps_testcases/1601_functions/160106_the_not_implemented_function/NegSem_160106_the_not_implemented_function_0...
- `core_language/16_functions_altsteps_testcases/1601_functions/160106_the_not_implemented_function/NegSem_160106_the_not_implemented_function_003.ttcn`: `error -> parse-error` - 1 error occurred:
	* {testdata/ttcn3-conformance-tests/ATS/core_language/16_functions_altsteps_testcases/1601_functions/160106_the_not_implemented_function/NegSem_160106_the_not_implemented_function_0...
- `core_language/16_functions_altsteps_testcases/1601_functions/160106_the_not_implemented_function/NegSem_160106_the_not_implemented_function_004.ttcn`: `error -> parse-error` - 14 errors occurred:
	* {testdata/ttcn3-conformance-tests/ATS/core_language/16_functions_altsteps_testcases/1601_functions/160106_the_not_implemented_function/NegSem_160106_the_not_implemented_function...
- `core_language/16_functions_altsteps_testcases/1601_functions/160106_the_not_implemented_function/NegSem_160106_the_not_implemented_function_005.ttcn`: `error -> parse-error` - 1 error occurred:
	* {testdata/ttcn3-conformance-tests/ATS/core_language/16_functions_altsteps_testcases/1601_functions/160106_the_not_implemented_function/NegSem_160106_the_not_implemented_function_0...
- `core_language/16_functions_altsteps_testcases/1601_functions/160106_the_not_implemented_function/NegSem_160106_the_not_implemented_function_007.ttcn`: `error -> parse-error` - 1 error occurred:
	* {testdata/ttcn3-conformance-tests/ATS/core_language/16_functions_altsteps_testcases/1601_functions/160106_the_not_implemented_function/NegSem_160106_the_not_implemented_function_0...
- `core_language/16_functions_altsteps_testcases/1601_functions/160106_the_not_implemented_function/NegSem_160106_the_not_implemented_function_008.ttcn`: `error -> parse-error` - 1 error occurred:
	* {testdata/ttcn3-conformance-tests/ATS/core_language/16_functions_altsteps_testcases/1601_functions/160106_the_not_implemented_function/NegSem_160106_the_not_implemented_function_0...
- `core_language/16_functions_altsteps_testcases/1601_functions/160106_the_not_implemented_function/Sem_160106_the_not_implemented_function_001.ttcn`: `pass -> parse-error` - 14 errors occurred:
	* {testdata/ttcn3-conformance-tests/ATS/core_language/16_functions_altsteps_testcases/1601_functions/160106_the_not_implemented_function/Sem_160106_the_not_implemented_function_00...
- `core_language/16_functions_altsteps_testcases/1601_functions/160106_the_not_implemented_function/Sem_160106_the_not_implemented_function_002.ttcn`: `pass -> parse-error` - 1 error occurred:
	* {testdata/ttcn3-conformance-tests/ATS/core_language/16_functions_altsteps_testcases/1601_functions/160106_the_not_implemented_function/Sem_160106_the_not_implemented_function_002....
- `core_language/16_functions_altsteps_testcases/1601_functions/160106_the_not_implemented_function/Sem_160106_the_not_implemented_function_003.ttcn`: `pass -> parse-error` - 1 error occurred:
	* {testdata/ttcn3-conformance-tests/ATS/core_language/16_functions_altsteps_testcases/1601_functions/160106_the_not_implemented_function/Sem_160106_the_not_implemented_function_003....
- `core_language/16_functions_altsteps_testcases/1601_functions/160106_the_not_implemented_function/Sem_160106_the_not_implemented_function_004.ttcn`: `pass -> parse-error` - 1 error occurred:
	* {testdata/ttcn3-conformance-tests/ATS/core_language/16_functions_altsteps_testcases/1601_functions/160106_the_not_implemented_function/Sem_160106_the_not_implemented_function_004....
- `core_language/16_functions_altsteps_testcases/1601_functions/1601_toplevel/NegSem_1601_toplevel_001.ttcn`: `reject -> pass`
- `core_language/16_functions_altsteps_testcases/1601_functions/1601_toplevel/Sem_1601_toplevel_004.ttcn`: `pass -> parse-error` - 3 errors occurred:
	* {testdata/ttcn3-conformance-tests/ATS/core_language/16_functions_altsteps_testcases/1601_functions/1601_toplevel/Sem_1601_toplevel_004.ttcn 8:14 8:21}: expected module definition...
- `core_language/16_functions_altsteps_testcases/1602_altsteps/160201_invoking_altsteps/NegSyn_160201_invoking_altsteps_001.ttcn`: ` -> `
- `core_language/16_functions_altsteps_testcases/1602_altsteps/1602_toplevel/Sem_1602_toplevel_004.ttcn`: `pass -> none`
- `core_language/16_functions_altsteps_testcases/1603_testcases/Syn_1603_testcases_004.ttcn`: `pass -> semantic-error` - testcase references component member "messagePort" (declared in component GeneralComp) but has no `runs on` clause (ETSI 16.1)

### `core_language/B_matching_incoming_values`

- `core_language/B_matching_incoming_values/B01_template_matching/B0102_matching_mechanisms/B010201_value_list/NegSem_B010201_value_list_001.ttcn`: `reject -> pass`
- `core_language/B_matching_incoming_values/B01_template_matching/B0102_matching_mechanisms/B010204_any_value_or_none/Sem_B010204_any_value_or_none_006.ttcn`: `pass -> fail`
- `core_language/B_matching_incoming_values/B01_template_matching/B0102_matching_mechanisms/B010204_any_value_or_none/Sem_B010204_any_value_or_none_007.ttcn`: `pass -> fail`
- `core_language/B_matching_incoming_values/B01_template_matching/B0102_matching_mechanisms/B010208_omit_value/Sem_B010208_omit_value_006.ttcn`: `pass -> fail`
- `core_language/B_matching_incoming_values/B01_template_matching/B0103_matching_inside_values/B010302_any_number_of_elements_or_none/Sem_B010302_any_number_of_elements_or_none_004.ttcn`: `pass -> error` - Error: array index must be an integer, got string
- `core_language/B_matching_incoming_values/B01_template_matching/B0103_matching_inside_values/B010302_any_number_of_elements_or_none/Sem_B010302_any_number_of_elements_or_none_005.ttcn`: `pass -> error` - Error: array index must be an integer, got string
- `core_language/B_matching_incoming_values/B01_template_matching/B0104_matching_attributes_of_values/B010402_ifPresent_indicator/NegSem_B010402_ifPresent_indicator_001.ttcn`: `reject -> none`
- `core_language/B_matching_incoming_values/B01_template_matching/B0104_matching_attributes_of_values/B010402_ifPresent_indicator/Sem_B010402_ifPresent_indicator_003.ttcn`: `pass -> fail`
- `core_language/B_matching_incoming_values/B01_template_matching/B0104_matching_attributes_of_values/B010402_ifPresent_indicator/Sem_B010402_ifPresent_indicator_005.ttcn`: `pass -> fail`
- `core_language/B_matching_incoming_values/B01_template_matching/B0105_matching_character_pattern/B010504_match_referenced_characters/NegSem_B010504_match_referenced_characters_001.ttcn`: `reject -> none`
