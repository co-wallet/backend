-- Apply after v11.sql. Synthetic API/browser fixture, no personal data.
UPDATE Account SET InitialBalanceCents=1000 WHERE Id='travel';
INSERT INTO Category VALUES ('unused','Unused',1,1,NULL,NULL);
INSERT INTO "Transaction" VALUES ('orphan','food','deleted',1000,638397504000000000,'Excluded synthetic operation',NULL,NULL);
-- Independent expected balances: Cash -3728.391 RUB; Travel 1412.520 TRY;
-- Reserve 1234.567 RUB. Five operations, including two distinct equal expenses.
