-- Seed for the SQL Server integration tests and e2e (M18 F18.6), the cases
-- of mysql.sql in T-SQL's terms. The mssql-seed service loads it, as sa,
-- only when sqlmux is not in the server's databases yet: the seed makes it
-- and goes there first. Values are fixed, not sysdatetime(), so screens and
-- assertions stay stable. GO batches: a create view and a create function
-- are a batch's first statement, and the second database's objects cannot
-- be made by a cross-database statement.

-- the filtered index below takes the session's QUOTED_IDENTIFIER ON, which
-- a bare sqlcmd session has off
set quoted_identifier on;
if db_id('sqlmux') is null create database sqlmux;
go
use sqlmux;
go

-- single-column primary key, an identity the server numbers
create table t_user (
    id         bigint identity(1, 1) not null primary key,
    name       nvarchar(64) not null,
    email      nvarchar(128),
    active     bit not null default 1,
    created_at datetime2 not null default sysdatetime()
);

-- over 5000 rows, for paging and counting; one column per type the grid
-- colours. SQL Server has no enum nor json: a check over a varchar, and
-- text the server reads with isjson.
create table t_order (
    id         bigint identity(1, 1) not null primary key,
    user_id    bigint not null,
    status     nvarchar(16) not null default 'pending' check (status in ('pending', 'queued', 'running', 'done', 'failed')),
    amount     decimal(10, 2) not null,
    paid       bit,
    meta       nvarchar(max),
    raw        nvarchar(max),
    note       nvarchar(max),
    created_at datetime2 not null default sysdatetime(),
    deleted_at datetime2 null,
    foreign key (user_id) references t_user (id)
);

-- composite primary key
create table t_order_item (
    order_id bigint not null,
    line_no  int not null,
    sku_code nvarchar(64) not null,
    qty      int not null default 1,
    primary key (order_id, line_no),
    foreign key (order_id) references t_order (id)
);

-- no primary key, only unique indexes. code and (title, code), keyed out of
-- column order, can identify a row; the nullable ones can't (§8.4). A
-- T-SQL unique index takes one null for all: barcode's is over the values
-- that are there, and so no stand-in (§10.1). MySQL's expression index has
-- no T-SQL shape of its own: no lower(title) index.
create table t_sku (
    code    nvarchar(64) not null,
    barcode nvarchar(64),
    title   nvarchar(64) not null,
    price   decimal(10, 2)
);
create unique index t_sku_code on t_sku (code);
create unique index t_sku_barcode on t_sku (barcode) where barcode is not null;
create unique index t_sku_title_code on t_sku (title, code);

-- no primary key and no unique index; one row carries control characters (§7.6)
create table t_log (
    logged_at datetime2 not null,
    msg       nvarchar(max)
);

go
create view v_paid_order as
    select id, user_id, amount from t_order where paid = 1;
go

-- the primary key's order is not the columns' order; MySQL's partitions have
-- no seed counterpart, the rows all there are
create table t_event (
    id          bigint not null,
    occurred_at datetime2 not null,
    kind        nvarchar(32) not null,
    primary key (occurred_at, id)
);

with s (i) as (select 1 union all select i + 1 from s where i < 50)
insert into t_user (name, email, active, created_at)
select 'user_' + cast(i as nvarchar),
       case when i % 5 = 0 then null else 'user_' + cast(i as nvarchar) + '@example.com' end,
       cast(case when i % 7 <> 0 then 1 else 0 end as bit),
       dateadd(day, i, '2026-01-01')
from s
option (maxrecursion 10000);

with s (i) as (select 1 union all select i + 1 from s where i < 6000)
insert into t_order (user_id, status, amount, paid, meta, raw, note, created_at, deleted_at)
select 1 + i % 50,
       case 1 + i % 4 when 1 then 'pending' when 2 then 'running' when 3 then 'done' else 'failed' end,
       (i % 1000) + 0.99,
       case when i % 3 = 0 then null else cast(i % 2 as bit) end,
       '{"n": ' + cast(i as nvarchar) + ', "tags": ["a", "b"]}',
       '{"n": ' + cast(i as nvarchar) + '}',
       case when i % 10 = 0 then 'note ' + cast(i as nvarchar) end,
       dateadd(minute, i, '2026-09-01'),
       case when i % 100 = 0 then '2026-09-20 12:00:00' end
from s
option (maxrecursion 10000);

with o (n) as (select 1 union all select n + 1 from o where n < 200),
     l (n) as (select 1 union all select n + 1 from l where n < 2)
insert into t_order_item (order_id, line_no, sku_code, qty)
select o.n, l.n, 'sku_' + cast((o.n + l.n) % 20 as nvarchar), 1 + o.n % 3
from o cross join l
option (maxrecursion 10000);

with s (i) as (select 0 union all select i + 1 from s where i < 19)
insert into t_sku (code, barcode, title, price)
select 'sku_' + cast(i as nvarchar),
       case when i % 4 = 0 then null else '69' + right(replicate('0', 11) + cast(i as nvarchar), 11) end,
       'SKU ' + cast(i as nvarchar),
       case when i % 5 = 0 then null else i * 10 end
from s
option (maxrecursion 10000);

insert into t_log (logged_at, msg) values
    ('2026-09-21 10:00:00', 'plain'),
    ('2026-09-21 10:01:00', 'line one' + char(10) + 'line two' + char(9) + 'after tab ' + char(27) + '[31mred' + char(27) + '[0m end'),
    ('2026-09-21 10:02:00', null);

insert into t_event (id, occurred_at, kind) values
    (1, '2025-06-01 00:00:00', 'signup'),
    (2, '2025-12-31 23:59:59', 'login'),
    (3, '2026-02-01 08:00:00', 'login');

-- M14 F14.3: a plain index and functions. A create function is a batch of
-- its own, as a create view is.
create index t_order_user_created on t_order (user_id, created_at);
go
create function dbo.fn_order_total (@order_id bigint) returns decimal(10, 2) as
begin
    return (select amount from t_order where id = @order_id)
end;
go
create function dbo.fn_greet (@who nvarchar(64)) returns nvarchar(80) as
begin
    return 'hello ' + @who
end;
go

-- the second database, as pg.sql's second schema and mysql.sql's agentable:
-- its objects take a database of their own to be made in
create database agentable;
go
use agentable;
go

create table dbo.agent (
    id   int identity(1, 1) not null primary key,
    name nvarchar(64) not null unique
);

create table dbo.agent_version (
    agent_id int not null,
    version  int not null,
    released bit not null default 0,
    primary key (agent_id, version),
    foreign key (agent_id) references dbo.agent (id)
);

create table dbo.goal (
    id       int identity(1, 1) not null primary key,
    agent_id int,
    title    nvarchar(64) not null,
    foreign key (agent_id) references dbo.agent (id)
);

insert into dbo.agent (name) values ('planner'), ('coder'), ('tester');
insert into dbo.agent_version (agent_id, version, released) values
    (1, 1, 1), (1, 2, 0), (2, 1, 1), (3, 1, 0);
insert into dbo.goal (agent_id, title) values
    (1, 'ship m1'), (2, 'write tests'), (null, 'unassigned');
go
create function dbo.fn_agent_count () returns int as
begin
    return (select count(*) from dbo.agent)
end;
go
use sqlmux;
go
