--
-- PostgreSQL database dump
--

-- Dumped from database version 18.1 (Debian 18.1-1.pgdg13+2)
-- Dumped by pg_dump version 18.1 (Debian 18.1-1.pgdg13+2)

--
-- Data for Name: kyb_data; Type: TABLE DATA; Schema: xcentro; Owner: xcentro_owner
--

INSERT INTO xcentro.kyb_data (income_tax_id, income_tax_id_data, company_id, company_id_data, business_deed) VALUES ('AAACH1004N', '{"pan": "AAACH1004N", "name": "", "status": "", "entity_type": "", "pan_com_response": {"data": {"name": "John Doe", "status": "Active", "category": "Individual", "is_valid": true, "pan_number": "ABCDE1234F", "aadhaar_linked": true}, "status": 1, "message": "PAN verification successful"}}', 'U70100DL2019PLC351211', '{"data": {"cin": "U12345KA2018PTC123456", "company_name": "ABC Private Limited", "company_status": "Active", "paid_up_capital": "5000000", "company_category": "Company limited by shares", "registration_date": "2018-01-15", "authorized_capital": "10000000", "registered_address": "123 Business Park, Bangalore - 560001", "company_sub_category": "Non-govt company"}, "status": 1, "message": "CIN verification successful"}', '')
ON CONFLICT DO NOTHING;


--
-- Data for Name: business; Type: TABLE DATA; Schema: xcentro; Owner: xcentro_owner
--

INSERT INTO xcentro.business (bid, name, gstin, gst_data, income_tax_id, kyb_status, business_type, business_model, business_turnover, country_code, currency_code, updated_at, has_added_payment_method, has_added_vendor, has_created_first_payment) VALUES ('019c52ec-8219-7952-89e5-07b0bb4ee713', 'Xcento''s Team', '36AAACH1004N3ZV', '{"data": {"gstin": "29ABCDE1234F1Z5", "state": "Karnataka", "status": "Active", "directors": [{"din": "12345678", "name": "John Doe"}], "legal_name": "ABC Private Limited", "trade_name": "ABC Corp", "business_type": "Private Limited Company", "taxpayer_type": "Regular", "principal_address": "123 Business Park, Bangalore - 560001", "registration_date": "2018-07-01"}, "status": 1, "message": "GSTIN verification successful"}', 'AAACH1004N', 'kyc_verified', 'private_limited_company', 'B2B (Business to Business)', '₹50 lakhs - ₹1 crore', NULL, NULL, '2026-02-12 17:58:20.106304+00', true, true, true)
ON CONFLICT DO NOTHING;


--
-- Data for Name: user_identity; Type: TABLE DATA; Schema: xcentro; Owner: xcentro_owner
--

INSERT INTO xcentro.user_identity (tax_id, tax_id_data, national_id, national_id_data, n_txn_id, created_at, updated_at) VALUES ('ABCDE1234Z', '{"pan_com_data": {"data": {"name": "John Doe", "status": "Active", "category": "Individual", "is_valid": true, "pan_number": "ABCDE1234F", "aadhaar_linked": true}, "status": 1, "message": "PAN verification successful"}, "pan_aadhaar_link_data": {"data": {"linked": true, "link_date": "2020-01-01", "pan_number": "ABCDE1234F", "aadhaar_number": "XXXX-XXXX-1234"}, "status": 1, "message": "PAN-Aadhaar link verified successfully"}}', '********7058', '{"data": {"dob": "01-01-1990", "name": "John Doe", "gender": "M", "address": "123 Mock Street, Mock City, Mock State - 123456", "verified": true, "photo_link": "https://mock-storage.example.com/photo.jpg", "verified_at": "2026-02-12T17:39:01Z", "aadhaar_number": "XXXX-XXXX-1234"}, "status": 1, "message": "Aadhaar verification completed successfully"}', 'TS_aadhaar_019c52e2-cb34-7b4a-87bf-da4585ff0812_1770917938_MOCK', '2026-02-12 17:38:58.707665+00', '2026-02-12 17:39:01.356573+00')
ON CONFLICT DO NOTHING;


--
-- Data for Name: user_table; Type: TABLE DATA; Schema: xcentro; Owner: xcentro_owner
--

INSERT INTO xcentro.user_table (id, name, dob, email, pwd_hash, mobile, tax_id, kyc_status, kyc_needed, email_verified, mobile_verified, created_at, updated_at) VALUES ('019c52e2-cb34-7b4a-87bf-da4585ff0812', 'Xcentro Admin', '2008-02-12', 'admin@acme.com', '$2a$12$iilGnBXX6LAVEi3uxkhtJe7T8mraJ5m5Q1opRZu.lWM2VUrBhKzEu', '', 'ABCDE1234Z', 'kyc_verified', true, true, true, '2026-02-12 17:25:20.817817+00', '2026-02-12 17:54:13.568523+00')
ON CONFLICT DO NOTHING;


--
-- Data for Name: payee_master; Type: TABLE DATA; Schema: xcentro; Owner: xcentro_owner
--

INSERT INTO xcentro.payee_master (payee_id, payee_type, is_active, created_at, updated_at) VALUES ('019c52fe-e912-7b7a-bf70-3d0f665d6a95', 'vendor', true, '2026-02-12 17:56:03.473943+00', NULL)
ON CONFLICT DO NOTHING;


--
-- Data for Name: business_payee_relation; Type: TABLE DATA; Schema: xcentro; Owner: xcentro_owner
--

INSERT INTO xcentro.business_payee_relation (business_id, payee_id, account_id, display_name, contact_email, contact_phone, notes, is_active, is_favorite, tags, type_specific_data, added_at, added_by, last_used_at, updated_at, metadata, relation_id) VALUES ('019c52ec-8219-7952-89e5-07b0bb4ee713', '019c52fe-e912-7b7a-bf70-3d0f665d6a95', '019c52fe-e918-7105-ba0d-7ac4d574d41e', 'Nexa Digital Solutions Pvt Ltd', 'accounts@nexadigital.com', '9876543210', '', true, false, '{}', NULL, '2026-02-12 17:56:03.473943+00', '019c52e2-cb34-7b4a-87bf-da4585ff0812', NULL, NULL, NULL, '93cbb482-e74a-4d85-b448-c89ff0837368')
ON CONFLICT DO NOTHING;


--
-- Data for Name: business_user_relation; Type: TABLE DATA; Schema: xcentro; Owner: xcentro_owner
--

INSERT INTO xcentro.business_user_relation (user_id, bid, role, role_claim, kyb_initiated, kyc_needed, added_at, updated_at) VALUES ('019c52e2-cb34-7b4a-87bf-da4585ff0812', '019c52ec-8219-7952-89e5-07b0bb4ee713', 'owner', 'owner', true, false, '2026-02-12 17:35:57.412468+00', '2026-02-12 17:54:08.56423+00')
ON CONFLICT DO NOTHING;


--
-- Data for Name: payments; Type: TABLE DATA; Schema: xcentro; Owner: xcentro_owner
--

INSERT INTO xcentro.payments (payment_id, user_id, business_id, amount, currency, payment_method, status, description, gateway_used, gateway_payment_id, callback_url, redirect_url, payment_source_id, payment_source_type, fee_amount, total_amount, metadata, created_at, updated_at, settlement_status, settled_at, settlement_reference_id, settlement_utr, settlement_metadata, settlement_amount, mdr_amount, net_settlement_amount, payment_type, retry_of_payment_id, retry_count, convenience_fee, convenience_fee_gst) VALUES ('019c52ff-6ee7-7b3b-864e-645e6b5a65f6', '019c52e2-cb34-7b4a-87bf-da4585ff0812', '019c52ec-8219-7952-89e5-07b0bb4ee713', 100, 'INR', 'card', 'failed', 'Card tokenization (₹1 verification charge)', 'mockpg', 'MOCKPG_019c52ff-6f56-7a1f-ad68-42a66735189d', 'http://localhost:8888/v1/process_payment/callback', 'http://localhost:8888/v1/process_payment/mock_3ds?transaction_id=019c52ff-6ee7-7b3b-864e-645e6b5a65f6&gateway_payment_id=MOCKPG_019c52ff-6f56-7a1f-ad68-42a66735189d&amount=102', '019c52ff-6ee9-75a5-b447-244665a00a2b', 'card', 2, 102, '{"gateway": "mockpg", "card_type": "visa", "merchant_id": "MOCK_MERCHANT_123", "processed_at": "2026-02-12T17:56:40.119Z", "callback_type": "payment_status", "card_category": "credit", "payment_method": "card", "mock_simulation": true, "callback_received_at": "2026-02-12T17:56:39Z", "tokenization_request": true, "mock_gateway_payment_id": "MOCKPG_019c52ff-6f56-7a1f-ad68-42a66735189d"}', '2026-02-12 17:56:37.736077+00', '2026-02-12 17:56:39.934479+00', 'pending', NULL, NULL, NULL, NULL, NULL, 0, NULL, 'tokenization', NULL, 0, 2, 0)
ON CONFLICT DO NOTHING;
INSERT INTO xcentro.payments (payment_id, user_id, business_id, amount, currency, payment_method, status, description, gateway_used, gateway_payment_id, callback_url, redirect_url, payment_source_id, payment_source_type, fee_amount, total_amount, metadata, created_at, updated_at, settlement_status, settled_at, settlement_reference_id, settlement_utr, settlement_metadata, settlement_amount, mdr_amount, net_settlement_amount, payment_type, retry_of_payment_id, retry_count, convenience_fee, convenience_fee_gst) VALUES ('019c5300-4d61-78b1-8b6e-423d4a49dc2f', '019c52e2-cb34-7b4a-87bf-da4585ff0812', '019c52ec-8219-7952-89e5-07b0bb4ee713', 100, 'INR', 'card', 'success', 'Card tokenization (₹1 verification charge)', 'mockpg', 'MOCKPG_019c5300-4dce-7c3f-b8be-c0310749ba73', 'http://localhost:8888/v1/process_payment/callback', 'http://localhost:8888/v1/process_payment/mock_3ds?transaction_id=019c5300-4d61-78b1-8b6e-423d4a49dc2f&gateway_payment_id=MOCKPG_019c5300-4dce-7c3f-b8be-c0310749ba73&amount=102', '019c5300-4d62-7294-ac93-f7eec1379438', 'card', 2, 102, '{"gateway": "mockpg", "card_type": "mastercard", "merchant_id": "MOCK_MERCHANT_123", "processed_at": "2026-02-12T17:57:37.089Z", "callback_type": "payment_status", "card_category": "credit", "payment_method": "card", "mock_simulation": true, "callback_received_at": "2026-02-12T17:57:36Z", "tokenization_request": true, "mock_gateway_payment_id": "MOCKPG_019c5300-4dce-7c3f-b8be-c0310749ba73"}', '2026-02-12 17:57:34.689807+00', '2026-02-12 17:57:36.70918+00', 'pending', NULL, NULL, NULL, NULL, NULL, 0, NULL, 'tokenization', NULL, 0, 2, 0)
ON CONFLICT DO NOTHING;
INSERT INTO xcentro.payments (payment_id, user_id, business_id, amount, currency, payment_method, status, description, gateway_used, gateway_payment_id, callback_url, redirect_url, payment_source_id, payment_source_type, fee_amount, total_amount, metadata, created_at, updated_at, settlement_status, settled_at, settlement_reference_id, settlement_utr, settlement_metadata, settlement_amount, mdr_amount, net_settlement_amount, payment_type, retry_of_payment_id, retry_count, convenience_fee, convenience_fee_gst) VALUES ('019c5300-fe4e-768b-8190-b070aba578a3', '019c52e2-cb34-7b4a-87bf-da4585ff0812', '019c52ec-8219-7952-89e5-07b0bb4ee713', 100000, 'INR', 'token_card', 'success', 'Payment for vouchers', 'mockpg', 'MOCKPG_019c5300-feb8-7aed-9d8b-449d3846a61d', 'http://localhost:8888/v1/process_payment/callback', 'http://localhost:8888/v1/process_payment/mock_3ds?transaction_id=019c5300-fe4e-768b-8190-b070aba578a3&gateway_payment_id=MOCKPG_019c5300-feb8-7aed-9d8b-449d3846a61d&amount=102596', '019c5300-4d62-7294-ac93-f7eec1379438', 'card', 2596, 102596, '{"gateway": "mockpg", "merchant_id": "MOCK_MERCHANT_123", "processed_at": "2026-02-12T17:58:21.979Z", "callback_type": "payment_status", "payment_method": "card", "mock_simulation": true, "callback_received_at": "2026-02-12T17:58:22Z", "mock_gateway_payment_id": "MOCKPG_019c5300-feb8-7aed-9d8b-449d3846a61d"}', '2026-02-12 17:58:19.98277+00', '2026-02-12 17:58:22.147576+00', 'pending', NULL, NULL, NULL, NULL, NULL, 0, NULL, 'payment', NULL, 0, 2200, 396)
ON CONFLICT DO NOTHING;


--
-- Data for Name: payment_vouchers; Type: TABLE DATA; Schema: xcentro; Owner: xcentro_owner
--

INSERT INTO xcentro.payment_vouchers (voucher_id, voucher_number, voucher_date, fiscal_year, accounting_period, voucher_type, business_id, payee_id, account_id, invoice_number, invoice_date, invoice_due_date, gross_amount, net_amount, currency, gst_amount, gst_rate, cgst_amount, sgst_amount, igst_amount, tds_amount, tds_rate, tds_section, fee_amount, discount_amount, financial_details, attachments, expense_category, description, payment_id, erp_sync_status, erp_reference_id, erp_posting_date, erp_last_sync_at, erp_sync_attempts, gl_expense_account, gl_bank_account, cost_center, erp_metadata, created_at, created_by_user_id, updated_at, metadata, payment_terms, status) VALUES ('019c5300-f26e-77e6-ba82-27f7d6c26359', 'PV-2025-00001', '2026-02-12', 'FY2025-26', '2026-02', 'payment', '019c52ec-8219-7952-89e5-07b0bb4ee713', '019c52fe-e912-7b7a-bf70-3d0f665d6a95', '019c52fe-e918-7105-ba0d-7ac4d574d41e', 'INV-001', '2026-02-12', NULL, 100000, 100000, 'INR', 0, 0.00, 0, 0, 0, 0, 0.00, '', 0, 0, '{"discount": {"type": "percentage", "value": 0, "amount": 0}, "gst_type": "cgst_sgst", "round_off": {"amount": 0, "applied": false}, "line_items": [{"rate": 100000, "amount": 100000, "gst_rate": 0, "quantity": 1, "gst_amount": 0, "description": "One week trail"}]}', 'null', '', 'One week trail', '019c5300-fe4e-768b-8190-b070aba578a3', 'not_synced', NULL, NULL, NULL, 0, '', '', '', NULL, '2026-02-12 17:58:16.880148+00', '019c52e2-cb34-7b4a-87bf-da4585ff0812', '2026-02-12 17:58:22.167487+00', 'null', NULL, 'finalized')
ON CONFLICT DO NOTHING;


--
-- Data for Name: notification_events; Type: TABLE DATA; Schema: xcentro; Owner: xcentro_owner
--

INSERT INTO xcentro.notification_events (id, event_type, payload, created_at) VALUES (1, 'user_role_approved', '{"target": {"type": "multicast", "business_id": "019c52ec-8219-7952-89e5-07b0bb4ee713", "minimum_role": "owner"}, "message": {"body": "User Xcentro Admin (admin@acme.com) has been approved with role: owner", "title": "User Approved"}, "entities": {"user_id": "019c52e2-cb34-7b4a-87bf-da4585ff0812"}, "timestamp": "2026-02-12T17:54:08Z", "event_type": "user_role_approved"}', '2026-02-12 17:54:08.578071+00')
ON CONFLICT DO NOTHING;
INSERT INTO xcentro.notification_events (id, event_type, payload, created_at) VALUES (2, 'kyb_verified', '{"target": {"type": "multicast", "business_id": "019c52ec-8219-7952-89e5-07b0bb4ee713", "minimum_role": "owner"}, "message": {"body": "Your business KYB verification has been approved!", "title": "KYB Verified"}, "entities": {}, "timestamp": "2026-02-12T17:54:13Z", "event_type": "kyb_verified"}', '2026-02-12 17:54:13.58348+00')
ON CONFLICT DO NOTHING;
INSERT INTO xcentro.notification_events (id, event_type, payload, created_at) VALUES (3, 'payments.completed', '{"target": {"type": "multicast", "business_id": "019c52ec-8219-7952-89e5-07b0bb4ee713", "minimum_role": "owner"}, "actions": [{"href": "/payments/019c5300-fe4e-768b-8190-b070aba578a3", "text": "View Details", "type": "button-neutral"}], "message": {"body": "Payment of ₹1025.96 has been processed successfully", "title": "Payment Successful"}, "entities": {"payment_id": "019c5300-fe4e-768b-8190-b070aba578a3", "business_id": "019c52ec-8219-7952-89e5-07b0bb4ee713"}, "timestamp": "2026-02-12T17:58:22Z", "event_type": "payments.completed"}', '2026-02-12 17:58:22.172051+00')
ON CONFLICT DO NOTHING;


--
-- Data for Name: ops_user_table; Type: TABLE DATA; Schema: xcentro; Owner: xcentro_owner
--

INSERT INTO xcentro.ops_user_table (id, name, email, roles, status, created_at, updated_at) VALUES ('00000000-0000-0000-0000-000000000001', 'Dev User', 'dev@xcentro.local', '["access.all"]', 'active', '2026-02-12 17:43:52.39786+00', '2026-02-12 17:43:52.39786+00')
ON CONFLICT DO NOTHING;


--
-- Data for Name: payee_bank_accounts; Type: TABLE DATA; Schema: xcentro; Owner: xcentro_owner
--

INSERT INTO xcentro.payee_bank_accounts (account_id, account_number_encrypted, account_number_hash, ifsc_code, bank_name, branch_name, account_type, account_holder_name, is_verified, verified_at, bank_verification_status, bank_verification_data, cross_verification_status, is_active, account_label, added_at, updated_at, metadata, castler_beneficiary_id) VALUES ('019c52fe-e918-7105-ba0d-7ac4d574d41e', 'LyKB7Po8lJZyeaGLGxxP6BitOnAy1rAFrsvA6QKq:Q/ExD6TfjdPeq7+w', '6cd59cbbe0a5cf96bd117f2054355e1058d032e214cc55c6750d50b078d6a30f', 'HDFC0000011', 'HDFC Bank', 'NEW DELHI - VASANT VIHAR', 'current', 'Nexa Digital Solutions Pvt Ltd', true, '2026-02-12 17:56:03.473749+00', 'success', '{"test": "test"}', 'success', true, '', '2026-02-12 17:56:03.473943+00', NULL, NULL, 'MOCK_PAYEE_7d1abc39')
ON CONFLICT DO NOTHING;


--
-- Data for Name: payee_vendor; Type: TABLE DATA; Schema: xcentro; Owner: xcentro_owner
--

INSERT INTO xcentro.payee_vendor (payee_id, gstin, legal_name, business_type, registration_date, gst_status, building_name, street, city, state, pincode, gst_verification_status, gst_verified_at, gst_data, created_at, updated_at, metadata) VALUES ('019c52fe-e912-7b7a-bf70-3d0f665d6a95', '27AAACN1234A1Z5', '', '', '0001-01-01', '', '', '', '', '', '', 'success', '2026-02-12 17:56:03.475031+00', '{"data": {"gstin": "29ABCDE1234F1Z5", "state": "Karnataka", "status": "Active", "directors": [{"din": "12345678", "name": "John Doe"}], "legal_name": "ABC Private Limited", "trade_name": "ABC Corp", "business_type": "Private Limited Company", "taxpayer_type": "Regular", "principal_address": "123 Business Park, Bangalore - 560001", "registration_date": "2018-07-01"}, "status": 1, "message": "GSTIN verification successful"}', '2026-02-12 17:56:03.473943+00', NULL, '{"directors": null, "state_code": ""}')
ON CONFLICT DO NOTHING;


--
-- Data for Name: payment_fee_rules; Type: TABLE DATA; Schema: xcentro; Owner: xcentro_owner
--

INSERT INTO xcentro.payment_fee_rules (rule_id, payment_method, card_type, card_category, fee_type, percentage_fee, fixed_fee, gst_percentage, gst_applicable, is_active, created_at, updated_at) VALUES ('019c52e2-753e-7525-a04d-494e00971563', 'card', 'mastercard', 'debit', 'percentage', 1.90, 0, 18.00, true, true, '2026-02-12 17:24:58.812792+00', '2026-02-12 17:24:58.812792+00')
ON CONFLICT DO NOTHING;
INSERT INTO xcentro.payment_fee_rules (rule_id, payment_method, card_type, card_category, fee_type, percentage_fee, fixed_fee, gst_percentage, gst_applicable, is_active, created_at, updated_at) VALUES ('019c52e2-753e-76db-8f31-18621e6f1d53', 'card', 'mastercard', 'credit', 'percentage', 2.20, 0, 18.00, true, true, '2026-02-12 17:24:58.812792+00', '2026-02-12 17:24:58.812792+00')
ON CONFLICT DO NOTHING;
INSERT INTO xcentro.payment_fee_rules (rule_id, payment_method, card_type, card_category, fee_type, percentage_fee, fixed_fee, gst_percentage, gst_applicable, is_active, created_at, updated_at) VALUES ('019c52e2-753e-76f6-963b-f6225e17c785', 'card', 'visa', 'debit', 'percentage', 1.90, 0, 18.00, true, true, '2026-02-12 17:24:58.812792+00', '2026-02-12 17:24:58.812792+00')
ON CONFLICT DO NOTHING;
INSERT INTO xcentro.payment_fee_rules (rule_id, payment_method, card_type, card_category, fee_type, percentage_fee, fixed_fee, gst_percentage, gst_applicable, is_active, created_at, updated_at) VALUES ('019c52e2-753e-7702-aff5-ed106641f6e6', 'card', 'visa', 'credit', 'percentage', 2.20, 0, 18.00, true, true, '2026-02-12 17:24:58.812792+00', '2026-02-12 17:24:58.812792+00')
ON CONFLICT DO NOTHING;
INSERT INTO xcentro.payment_fee_rules (rule_id, payment_method, card_type, card_category, fee_type, percentage_fee, fixed_fee, gst_percentage, gst_applicable, is_active, created_at, updated_at) VALUES ('019c52e2-753e-77e8-87ff-445053cccdcd', 'net_banking', NULL, NULL, 'fixed', NULL, 2800, 18.00, true, true, '2026-02-12 17:24:58.812792+00', '2026-02-12 17:24:58.812792+00')
ON CONFLICT DO NOTHING;
INSERT INTO xcentro.payment_fee_rules (rule_id, payment_method, card_type, card_category, fee_type, percentage_fee, fixed_fee, gst_percentage, gst_applicable, is_active, created_at, updated_at) VALUES ('019c52e2-753e-7853-b694-da9f59bcbae1', 'upi', NULL, NULL, 'fixed', NULL, 500, 18.00, true, true, '2026-02-12 17:24:58.812792+00', '2026-02-12 17:24:58.812792+00')
ON CONFLICT DO NOTHING;
INSERT INTO xcentro.payment_fee_rules (rule_id, payment_method, card_type, card_category, fee_type, percentage_fee, fixed_fee, gst_percentage, gst_applicable, is_active, created_at, updated_at) VALUES ('019c52e2-75ce-7ce5-bb01-39656631f319', 'card', 'rupay', 'credit', 'percentage', 0.90, 0, 18.00, true, true, '2026-02-12 17:24:58.95723+00', '2026-02-12 17:24:58.95723+00')
ON CONFLICT DO NOTHING;
INSERT INTO xcentro.payment_fee_rules (rule_id, payment_method, card_type, card_category, fee_type, percentage_fee, fixed_fee, gst_percentage, gst_applicable, is_active, created_at, updated_at) VALUES ('019c52e2-75ce-7e02-b26d-6bb55eadede4', 'card', 'rupay', 'debit', 'percentage', 0.90, 0, 18.00, true, true, '2026-02-12 17:24:58.95723+00', '2026-02-12 17:24:58.95723+00')
ON CONFLICT DO NOTHING;
INSERT INTO xcentro.payment_fee_rules (rule_id, payment_method, card_type, card_category, fee_type, percentage_fee, fixed_fee, gst_percentage, gst_applicable, is_active, created_at, updated_at) VALUES ('019c52e2-75ce-7e1b-a697-58ac2df67a32', 'card', 'rupay', 'corporate', 'percentage', 0.90, 0, 18.00, true, true, '2026-02-12 17:24:58.95723+00', '2026-02-12 17:24:58.95723+00')
ON CONFLICT DO NOTHING;
INSERT INTO xcentro.payment_fee_rules (rule_id, payment_method, card_type, card_category, fee_type, percentage_fee, fixed_fee, gst_percentage, gst_applicable, is_active, created_at, updated_at) VALUES ('019c52e2-75ce-7e2a-aaef-df5f5666b60e', 'card', 'amex', 'credit', 'percentage', 3.00, 0, 18.00, true, true, '2026-02-12 17:24:58.95723+00', '2026-02-12 17:24:58.95723+00')
ON CONFLICT DO NOTHING;
INSERT INTO xcentro.payment_fee_rules (rule_id, payment_method, card_type, card_category, fee_type, percentage_fee, fixed_fee, gst_percentage, gst_applicable, is_active, created_at, updated_at) VALUES ('019c52e2-75ce-7e3a-96d0-5009a8f79d67', 'card', 'amex', 'debit', 'percentage', 3.00, 0, 18.00, true, true, '2026-02-12 17:24:58.95723+00', '2026-02-12 17:24:58.95723+00')
ON CONFLICT DO NOTHING;
INSERT INTO xcentro.payment_fee_rules (rule_id, payment_method, card_type, card_category, fee_type, percentage_fee, fixed_fee, gst_percentage, gst_applicable, is_active, created_at, updated_at) VALUES ('019c52e2-75ce-7e45-902e-b1669edbd552', 'card', 'amex', 'corporate', 'percentage', 3.00, 0, 18.00, true, true, '2026-02-12 17:24:58.95723+00', '2026-02-12 17:24:58.95723+00')
ON CONFLICT DO NOTHING;
INSERT INTO xcentro.payment_fee_rules (rule_id, payment_method, card_type, card_category, fee_type, percentage_fee, fixed_fee, gst_percentage, gst_applicable, is_active, created_at, updated_at) VALUES ('019c52e2-75ce-7e51-9c02-169cc3c526bf', 'card', 'diners', 'credit', 'percentage', 3.00, 0, 18.00, true, true, '2026-02-12 17:24:58.95723+00', '2026-02-12 17:24:58.95723+00')
ON CONFLICT DO NOTHING;
INSERT INTO xcentro.payment_fee_rules (rule_id, payment_method, card_type, card_category, fee_type, percentage_fee, fixed_fee, gst_percentage, gst_applicable, is_active, created_at, updated_at) VALUES ('019c52e2-75ce-7e5c-bdc9-1577c85af5cb', 'card', 'diners', 'debit', 'percentage', 3.00, 0, 18.00, true, true, '2026-02-12 17:24:58.95723+00', '2026-02-12 17:24:58.95723+00')
ON CONFLICT DO NOTHING;
INSERT INTO xcentro.payment_fee_rules (rule_id, payment_method, card_type, card_category, fee_type, percentage_fee, fixed_fee, gst_percentage, gst_applicable, is_active, created_at, updated_at) VALUES ('019c52e2-75ce-7e68-9eb9-69ea949f1073', 'card', 'diners', 'corporate', 'percentage', 3.00, 0, 18.00, true, true, '2026-02-12 17:24:58.95723+00', '2026-02-12 17:24:58.95723+00')
ON CONFLICT DO NOTHING;


--
-- Data for Name: payment_invoices; Type: TABLE DATA; Schema: xcentro; Owner: xcentro_owner
--

INSERT INTO xcentro.payment_invoices (invoice_id, payment_id, invoice_number, gstin_supplier, gstin_recipient, invoice_date, irn, irn_ack_no, irn_ack_date, irn_status, qr_code, filing_status, description, sac_code, taxable_value, gst_rate, gst_amount, total_amount, created_at, updated_at) VALUES (1, '019c5300-4d61-78b1-8b6e-423d4a49dc2f', '', '27AATCM5865F1ZU', '36AAACH1004N3ZV', '2026-02-12', '5f39132427c8a2ddb398d7fca1c95950e6df76ac31aafb069e76f833b1b6f819', '', NULL, 'pending', NULL, 'not_filed', 'Convenience Fee', '997159', 0.02, 18.00, 0.00, 0.02, '2026-02-12 17:57:36.730168+00', '2026-02-12 17:57:36.732824+00')
ON CONFLICT DO NOTHING;
INSERT INTO xcentro.payment_invoices (invoice_id, payment_id, invoice_number, gstin_supplier, gstin_recipient, invoice_date, irn, irn_ack_no, irn_ack_date, irn_status, qr_code, filing_status, description, sac_code, taxable_value, gst_rate, gst_amount, total_amount, created_at, updated_at) VALUES (2, '019c5300-fe4e-768b-8190-b070aba578a3', '', '27AATCM5865F1ZU', '36AAACH1004N3ZV', '2026-02-12', '9b8eafabce1e65224052d26c7cde903afc46cfa6a0926705497e36a11b084ed5', '', NULL, 'pending', NULL, 'not_filed', 'Convenience Fee', '997159', 22.00, 18.00, 3.96, 25.96, '2026-02-12 17:58:22.161049+00', '2026-02-12 17:58:22.164528+00')
ON CONFLICT DO NOTHING;


--
-- Data for Name: payment_state_changes; Type: TABLE DATA; Schema: xcentro; Owner: xcentro_owner
--

INSERT INTO xcentro.payment_state_changes (state_change_id, payment_id, transfer_request_id, action, action_timestamp, actor_type, actor_user_id, actor_name, previous_state, new_state, amount, currency, utr, provider, metadata, remarks, created_at) VALUES ('019c52ff-6f5f-74e7-b9de-45cb5644d36f', '019c52ff-6ee7-7b3b-864e-645e6b5a65f6', NULL, 'payment_initiated', '2026-02-12 17:56:37.855308+00', 'user', '019c52e2-cb34-7b4a-87bf-da4585ff0812', NULL, NULL, NULL, 100, 'INR', NULL, 'mockpg', NULL, 'Payment initiated via mockpg gateway', '2026-02-12 17:56:37.736077+00')
ON CONFLICT DO NOTHING;
INSERT INTO xcentro.payment_state_changes (state_change_id, payment_id, transfer_request_id, action, action_timestamp, actor_type, actor_user_id, actor_name, previous_state, new_state, amount, currency, utr, provider, metadata, remarks, created_at) VALUES ('019c52ff-7780-7d75-8805-c5f736c1c4ed', '019c52ff-6ee7-7b3b-864e-645e6b5a65f6', NULL, 'payment_failed', '2026-02-12 17:56:39.93687+00', 'webhook', NULL, 'mockpg_webhook', NULL, 'failed', NULL, NULL, NULL, 'mockpg', '{"gateway": "mockpg", "merchant_id": "MOCK_MERCHANT_123", "processed_at": "2026-02-12T17:56:40.119Z", "callback_type": "payment_status", "payment_method": "card", "mock_simulation": true, "callback_received_at": "2026-02-12T17:56:39Z", "mock_gateway_payment_id": "MOCKPG_019c52ff-6f56-7a1f-ad68-42a66735189d"}', 'Payment failed: Payment failed', '2026-02-12 17:56:39.939413+00')
ON CONFLICT DO NOTHING;
INSERT INTO xcentro.payment_state_changes (state_change_id, payment_id, transfer_request_id, action, action_timestamp, actor_type, actor_user_id, actor_name, previous_state, new_state, amount, currency, utr, provider, metadata, remarks, created_at) VALUES ('019c5300-4dd3-7b9e-a2b4-d8d3b9e85ca1', '019c5300-4d61-78b1-8b6e-423d4a49dc2f', NULL, 'payment_initiated', '2026-02-12 17:57:34.803753+00', 'user', '019c52e2-cb34-7b4a-87bf-da4585ff0812', NULL, NULL, NULL, 100, 'INR', NULL, 'mockpg', NULL, 'Payment initiated via mockpg gateway', '2026-02-12 17:57:34.689807+00')
ON CONFLICT DO NOTHING;
INSERT INTO xcentro.payment_state_changes (state_change_id, payment_id, transfer_request_id, action, action_timestamp, actor_type, actor_user_id, actor_name, previous_state, new_state, amount, currency, utr, provider, metadata, remarks, created_at) VALUES ('019c5300-5548-75fa-9763-d5469788d4e2', '019c5300-4d61-78b1-8b6e-423d4a49dc2f', NULL, 'payment_success', '2026-02-12 17:57:36.712376+00', 'webhook', NULL, 'mockpg_webhook', NULL, 'success', 102, 'INR', NULL, 'mockpg', '{"gateway": "mockpg", "merchant_id": "MOCK_MERCHANT_123", "processed_at": "2026-02-12T17:57:37.089Z", "callback_type": "payment_status", "payment_method": "card", "mock_simulation": true, "callback_received_at": "2026-02-12T17:57:36Z", "mock_gateway_payment_id": "MOCKPG_019c5300-4dce-7c3f-b8be-c0310749ba73"}', 'Payment successful. Gateway Payment ID: MOCKPG_019c5300-4dce-7c3f-b8be-c0310749ba73', '2026-02-12 17:57:36.716338+00')
ON CONFLICT DO NOTHING;
INSERT INTO xcentro.payment_state_changes (state_change_id, payment_id, transfer_request_id, action, action_timestamp, actor_type, actor_user_id, actor_name, previous_state, new_state, amount, currency, utr, provider, metadata, remarks, created_at) VALUES ('019c5300-febc-7df2-a07e-1acb2f28bdcc', '019c5300-fe4e-768b-8190-b070aba578a3', NULL, 'payment_initiated', '2026-02-12 17:58:20.092906+00', 'user', '019c52e2-cb34-7b4a-87bf-da4585ff0812', NULL, NULL, NULL, 100000, 'INR', NULL, 'mockpg', NULL, 'Payment initiated via mockpg gateway', '2026-02-12 17:58:19.98277+00')
ON CONFLICT DO NOTHING;
INSERT INTO xcentro.payment_state_changes (state_change_id, payment_id, transfer_request_id, action, action_timestamp, actor_type, actor_user_id, actor_name, previous_state, new_state, amount, currency, utr, provider, metadata, remarks, created_at) VALUES ('019c5301-06c7-7ea4-8a2b-ba4b350883b2', '019c5300-fe4e-768b-8190-b070aba578a3', NULL, 'payment_success', '2026-02-12 17:58:22.151949+00', 'webhook', NULL, 'mockpg_webhook', NULL, 'success', 102596, 'INR', NULL, 'mockpg', '{"gateway": "mockpg", "merchant_id": "MOCK_MERCHANT_123", "processed_at": "2026-02-12T17:58:21.979Z", "callback_type": "payment_status", "payment_method": "card", "mock_simulation": true, "callback_received_at": "2026-02-12T17:58:22Z", "mock_gateway_payment_id": "MOCKPG_019c5300-feb8-7aed-9d8b-449d3846a61d"}', 'Payment successful. Gateway Payment ID: MOCKPG_019c5300-feb8-7aed-9d8b-449d3846a61d', '2026-02-12 17:58:22.154713+00')
ON CONFLICT DO NOTHING;


--
-- Data for Name: stored_cards; Type: TABLE DATA; Schema: xcentro; Owner: xcentro_owner
--

INSERT INTO xcentro.stored_cards (card_id, business_id, token, last_four_digits, card_type, card_category, expiry_month, expiry_year, issuer_bank, card_holder_name, is_active, gateway_used, added_at, last_used_at, metadata, failure_reason, is_failed_tokenization) VALUES ('019c52ff-6ee9-75a5-b447-244665a00a2b', '019c52ec-8219-7952-89e5-07b0bb4ee713', '', '1111', 'visa', 'credit', '01', '30', '', 'Admin', false, NULL, '2026-02-12 17:56:37.736077+00', NULL, '{"phase": "1_partial_data", "status": "pending_tokenization"}', 'Payment failed', true)
ON CONFLICT DO NOTHING;
INSERT INTO xcentro.stored_cards (card_id, business_id, token, last_four_digits, card_type, card_category, expiry_month, expiry_year, issuer_bank, card_holder_name, is_active, gateway_used, added_at, last_used_at, metadata, failure_reason, is_failed_tokenization) VALUES ('019c5300-4d62-7294-ac93-f7eec1379438', '019c52ec-8219-7952-89e5-07b0bb4ee713', 'MOCK_TOKEN_019c5300-55b2-7a57-bfcf-5259ff4a5977', '8785', 'mastercard', 'credit', '01', '30', 'Unknown', 'Admin', true, 'mockpg', '2026-02-12 17:57:34.689807+00', NULL, '{"phase": "2_tokenization_complete", "status": "active", "gateway_card_id": "MOCK_CARD_019c5300-55b2-7a", "tokenization_date": "2026-02-12"}', NULL, false)
ON CONFLICT DO NOTHING;


--
-- Data for Name: user_tokens; Type: TABLE DATA; Schema: xcentro; Owner: xcentro_owner
--

INSERT INTO xcentro.user_tokens (session_id, user_id, refresh_token_hash, expires_at, created_at, last_used_at, user_agent, device_type, device_version) VALUES ('280b72fe-6c18-4fad-9998-deff1f9dcadb', '019c52e2-cb34-7b4a-87bf-da4585ff0812', 'f3e303d106d9705c5e8161e0ce7964b1bf17ff56c9d58d722e71a518ebfb81c2', '2026-03-14 17:27:01.342402+00', '2026-02-12 17:27:01.332695+00', '2026-02-12 17:27:01.332695+00', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36 Edg/144.0.0.0', 'web', 1)
ON CONFLICT DO NOTHING;
INSERT INTO xcentro.user_tokens (session_id, user_id, refresh_token_hash, expires_at, created_at, last_used_at, user_agent, device_type, device_version) VALUES ('318f925b-6142-40ae-9c06-86f83caa1384', '019c52e2-cb34-7b4a-87bf-da4585ff0812', 'e6b9c121ce99ea46e79d507d003b4beca822eab0093bdfbaac7c26cb6f28b2e3', '2026-03-14 17:29:17.005101+00', '2026-02-12 17:29:17.002158+00', '2026-02-12 17:29:17.002158+00', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36 Edg/144.0.0.0', 'web', 1)
ON CONFLICT DO NOTHING;
INSERT INTO xcentro.user_tokens (session_id, user_id, refresh_token_hash, expires_at, created_at, last_used_at, user_agent, device_type, device_version) VALUES ('8412fa45-b131-4916-b8ef-fd066bb46282', '019c52e2-cb34-7b4a-87bf-da4585ff0812', 'b615fcbc68254dd1efde0db9dc8b2cb93c9291f1b43a5f1bbd46ee9b734d66d4', '2026-03-14 17:35:38.041056+00', '2026-02-12 17:35:38.037064+00', '2026-02-12 17:35:38.037064+00', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36 Edg/144.0.0.0', 'web', 1)
ON CONFLICT DO NOTHING;


--
-- Data for Name: user_verification_codes; Type: TABLE DATA; Schema: xcentro; Owner: xcentro_owner
--

INSERT INTO xcentro.user_verification_codes (id, user_id, code, code_type, expires_at, created_at, used, retry_attempts) VALUES ('019c52e2-cb3a-74c6-938f-efacdbbffec5', '019c52e2-cb34-7b4a-87bf-da4585ff0812', '5ae0b2500d569224458d7bb9caacdb10ac62b55e46f9bb462ad57605242b3516', 'email_verification', '2026-02-13 17:25:20+00', '2026-02-12 17:25:20+00', false, 0)
ON CONFLICT DO NOTHING;


--
-- PostgreSQL database dump complete
--
