# Current Conformance Misses

Generated from `etsi.json` using the current workspace runner.

- Summary: `ran 5164 files: 5003 matched, 21 skipped (97.28% match rate)`
- Misses: `161`

## Misses By Outcome

- `reject->pass`: `65`
- `reject->none`: `27`
- `->`: `21`
- `pass->fail`: `14`
- `pass->parse-error`: `13`
- `pass->error`: `11`
- `pass->semantic-error`: `7`
- `pass->none`: `2`
- `error->parse-error`: `1`

## Misses By Chapter

- `core_language/06_types_and_values`: `59` (`->`=8, `pass->error`=6, `pass->fail`=5, `pass->none`=1, `pass->parse-error`=6, `pass->semantic-error`=2, `reject->none`=13, `reject->pass`=18)
- `core_language/22_communication_operations`: `20` (`reject->none`=1, `reject->pass`=19)
- `core_language/15_templates`: `18` (`->`=2, `pass->error`=2, `pass->fail`=4, `reject->none`=3, `reject->pass`=7)
- `core_language/05_basic_language_elements`: `16` (`->`=2, `pass->parse-error`=2, `reject->none`=1, `reject->pass`=11)
- `core_language/B_matching_incoming_values`: `10` (`pass->error`=2, `pass->fail`=5, `reject->none`=2, `reject->pass`=1)
- `core_language/16_functions_altsteps_testcases`: `9` (`->`=1, `error->parse-error`=1, `pass->parse-error`=2, `pass->semantic-error`=1, `reject->none`=1, `reject->pass`=3)
- `core_language/09_test_configurations`: `8` (`pass->error`=1, `reject->none`=4, `reject->pass`=3)
- `core_language/07_expressions`: `6` (`pass->semantic-error`=4, `reject->pass`=2)
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
- `core_language/06_types_and_values/0602_structured_types_and_values/060201_record_type_and_values/06020104_embedding_fields/Sem_06020104_embedding_fields_002.ttcn`: `pass -> parse-error` - 2 errors occurred:
	* testdata/ttcn3-conformance-tests/ATS/core_language/06_types_and_values/0602_structured_types_and_values/060201_record_type_and_values/06020104_embedding_fields/Sem_06020104_embed...
- `core_language/06_types_and_values/0602_structured_types_and_values/060201_record_type_and_values/06020104_embedding_fields/Sem_06020104_embedding_fields_003.ttcn`: `pass -> fail` - field c not promoted
- `core_language/06_types_and_values/0602_structured_types_and_values/060201_record_type_and_values/06020104_embedding_fields/Sem_06020104_embedding_fields_004.ttcn`: `pass -> fail`
- `core_language/06_types_and_values/0602_structured_types_and_values/060201_record_type_and_values/060201_toplevel/NegSem_060201_RecordTypeValues_001.ttcn`: `reject -> none`
- `core_language/06_types_and_values/0602_structured_types_and_values/060202_set_type_and_values/NegSem_060202_SetTypeValues_001.ttcn`: `reject -> none`
- `core_language/06_types_and_values/0602_structured_types_and_values/060202_set_type_and_values/NegSem_060202_SetTypeValues_002.ttcn`: `reject -> none`
- `core_language/06_types_and_values/0602_structured_types_and_values/060203_records_and_sets_of_single_types/NegSem_060203_records_and_sets_of_single_types_001.ttcn`: `reject -> none`
- `core_language/06_types_and_values/0602_structured_types_and_values/060203_records_and_sets_of_single_types/NegSem_060203_records_and_sets_of_single_types_002.ttcn`: `reject -> none`
- `core_language/06_types_and_values/0602_structured_types_and_values/060205_unions/06020501_referencing_fields_of_union_type/NegSem_06020501_referencing_fields_of_union_type_005.ttcn`: `reject -> pass`
- `core_language/06_types_and_values/0602_structured_types_and_values/060205_unions/06020502_option_and_union/NegSyn_06020502_option_and_union_001.ttcn`: ` -> `
- `core_language/06_types_and_values/0602_structured_types_and_values/060205_unions/06020504_embedding_fields/Sem_06020504_embedding_fields_001.ttcn`: `pass -> fail` - a.R is not chosen
- `core_language/06_types_and_values/0602_structured_types_and_values/060206_anytype/NegSem_060206_anytype_001.ttcn`: `reject -> pass`
- `core_language/06_types_and_values/0602_structured_types_and_values/060207_arrays/NegSem_060207_arrays_026.ttcn`: `reject -> pass`
- `core_language/06_types_and_values/0602_structured_types_and_values/060207_arrays/NegSem_060207_arrays_027.ttcn`: `reject -> pass`
- `core_language/06_types_and_values/0602_structured_types_and_values/060207_arrays/NegSem_060207_arrays_028.ttcn`: `reject -> pass`
- ... `39` more in `current-misses.json`

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

### `core_language/15_templates`

- `core_language/15_templates/1503_global_and_local_templates/NegSyn_1503_GlobalAndLocalTemplates_006.ttcn`: ` -> `
- `core_language/15_templates/1506_referencing_elements_of_templates_or_template_fields/150601_referencing_individual_string_elements/Sem_150601_ReferencingIndividualStringElements_003.ttcn`: `pass -> fail`
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
- `core_language/15_templates/1513_the_present_operation/NegSem_1512_ThePresentOperation_003.ttcn`: `reject -> pass`
- `core_language/15_templates/15_toplevel/NegSyn_15_TopLevel_001.ttcn`: ` -> `

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
- `core_language/05_basic_language_elements/0504_parametrization/050403_variadic_parameters/Sem_050403_variadic_parameters_009.ttcn`: `pass -> parse-error` - 1 error occurred:
	* testdata/ttcn3-conformance-tests/ATS/core_language/05_basic_language_elements/0504_parametrization/050403_variadic_parameters/Sem_050403_variadic_parameters_009.ttcn:9:32-11:2: ex...
- `core_language/05_basic_language_elements/0504_parametrization/050403_variadic_parameters/Sem_050403_variadic_parameters_011.ttcn`: `pass -> parse-error` - 6 errors occurred:
	* testdata/ttcn3-conformance-tests/ATS/core_language/05_basic_language_elements/0504_parametrization/050403_variadic_parameters/Sem_050403_variadic_parameters_011.ttcn:9:34-9:35: e...
- `core_language/05_basic_language_elements/0504_parametrization/0504_toplevel/NegSyn_0504_forbidden_parametrization_001.ttcn`: `reject -> none`
- `core_language/05_basic_language_elements/0505_cyclic_definitions/NegSem_0505_cyclic_definitions_001.ttcn`: ` -> `
- `core_language/05_basic_language_elements/05_toplevel/NegSyn_05_TopLevel_001.ttcn`: ` -> `

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
