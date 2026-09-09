-- Synthetic data only. Amounts are thousandths; dates are .NET ticks.
PRAGMA user_version = 11;
CREATE TABLE Account (Id TEXT PRIMARY KEY, Name TEXT, Icon INTEGER, CreatedOn INTEGER, InitialBalanceCents INTEGER, IsIncludedInTotalBalance INTEGER, CurrencyId INTEGER, DisabledOn INTEGER, DeletedOn INTEGER);
CREATE TABLE Category (Id TEXT PRIMARY KEY, Name TEXT, CategoryType INTEGER, Icon INTEGER, DisabledOn INTEGER, DeletedOn INTEGER);
CREATE TABLE "Transaction" (Id TEXT PRIMARY KEY, CategoryId TEXT, AccountId TEXT, AmountCents INTEGER, CreatedOn INTEGER, Note TEXT, ScheduleId TEXT, DeletedOn INTEGER);
CREATE TABLE Transfer (Id TEXT PRIMARY KEY, AccountFromId TEXT, AccountToId TEXT, AmountCents INTEGER, CreatedOn INTEGER, Note TEXT, DeletedOn INTEGER);
CREATE TABLE Currency (Id INTEGER PRIMARY KEY, Name TEXT, AlphabeticCode TEXT, NumericCode INTEGER, MinorUnits INTEGER, IsBase INTEGER, Symbol TEXT);
CREATE TABLE CurrencyRate (Id TEXT PRIMARY KEY, CurrencyFromId INTEGER, CurrencyToId INTEGER, RateCents INTEGER, RateDate INTEGER, CreatedOn INTEGER, DeletedOn INTEGER);
CREATE TABLE Schedule (Id TEXT PRIMARY KEY, ReminderType INTEGER, CreatedOn INTEGER, StartOn INTEGER, EndOn INTEGER, EntityId TEXT, ScheduleType INTEGER, ExtendedData INTEGER, DeletedOn INTEGER);
CREATE TABLE Setting (Id TEXT PRIMARY KEY, Value TEXT);
INSERT INTO Currency VALUES (643, 'Ruble', 'RUB', 643, 2, 1, 'R'), (949, 'Lira', 'TRY', 949, 2, 0, 'T');
INSERT INTO Account VALUES
 ('cash', 'Cash', 7, 638396640000000001, -1234567, 1, 643, NULL, NULL),
 ('travel', 'Travel', 9, 638396640000000000, 9007199254740993, 0, 949, 638397504000000000, NULL),
 ('reserve', 'Reserve', 1, 638396640000000000, 0, 1, 643, NULL, NULL),
 ('deleted', 'Deleted', 1, 638396640000000000, 0, 1, 643, NULL, 638397504000000000);
INSERT INTO Category VALUES ('food', 'Food', 1, 12, NULL, NULL), ('pay', 'Pay', 0, 3, NULL, NULL), ('old', 'Old', 1, 1, NULL, 638397504000000000);
INSERT INTO "Transaction" VALUES
 ('expense-1', 'food', 'cash', 12345, 638397504000000007, 'Identical note', NULL, NULL),
 ('expense-2', 'food', 'cash', 12345, 638397504000000007, 'Identical note', NULL, NULL),
 ('income', 'pay', 'travel', 999999, 638397504000000000, NULL, NULL, NULL),
 ('removed', 'food', 'cash', 1000, 638397504000000000, NULL, NULL, 638397504000000000);
INSERT INTO Transfer VALUES
 ('fx', 'cash', 'travel', 1234567, 638398368000000000, NULL, NULL),
 ('same', 'cash', 'reserve', 1234567, 638398368000000000, 'Move', NULL),
 ('removed', 'cash', 'reserve', 10, 638398368000000000, NULL, 638398368000000000);
INSERT INTO CurrencyRate VALUES
 ('earlier', 643, 949, 100000, 638396640000000000, 638396640000000000, NULL),
 ('revision-1', 643, 949, 200000, 638397504000000000, 638398368000000000, NULL),
 ('revision-2', 643, 949, 333333, 638397504000000000, 638399232000000000, NULL),
 ('future', 643, 949, 500000, 638399232000000000, 638399232000000000, NULL),
 ('removed', 643, 949, 900000, 638398368000000000, 638399232000000000, 638399232000000000);
