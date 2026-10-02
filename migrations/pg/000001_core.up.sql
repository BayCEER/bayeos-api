-- Node 
create type node_path as (path text, ids int[]);

-- Required by the gin_trgm_ops index on node path below
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE node (
    id serial primary key,
    display_name text not null,
    kind text not null default 'Folder',    
    parent_id int,
    path node_path null, 
    metadata jsonb NOT NULL DEFAULT '{}',
    created_at timestamp with time zone default now(),      
    CONSTRAINT uk_node_display_name_kind UNIQUE (parent_id,display_name,kind)    
);

create table node_token (
        id serial primary key,
        node_id int not null,
        active bool default true,
        token uuid not null
);


CREATE TABLE user_group (
    id serial primary key,
    display_name text not null unique,
    sys_group bool not null default false,
    created_at timestamp with time zone default now()
);

CREATE TABLE users (
    id serial primary key,
    display_name text not null,
    sign_in_name text not null ,
    email_address text not null default '',
    home_node_id int not null default 2,    
    password_hash text not null default '',
    super_user bool default false,
    sys_user bool default false,
    last_sign_in_time timestamp with time zone,
    created_at timestamp with time zone not null default now(),
    constraint uk_user_sign_in_name unique (sign_in_name)
);  

CREATE TABLE user_group_member (
    id serial primary key,
    group_id int not null,
    user_id int not null,
    CONSTRAINT uk_user_group_member UNIQUE (group_id, user_id)
);

CREATE TABLE user_images (
 id SERIAL PRIMARY KEY,
 user_id INTEGER NOT NULL,
 image_data BYTEA NOT NULL, 
 mime_type TEXT NOT NULL DEFAULT 'image/jpeg'
);

CREATE TABLE user_chart (
 id SERIAL PRIMARY KEY,
 user_id INTEGER NOT NULL,
 display_name TEXT NOT NULL,
 description TEXT NOT NULL DEFAULT '',
 created_at timestamp with time zone not null default now() 
 );

CREATE TABLE user_chart_series (
    id SERIAL PRIMARY KEY,
    user_chart_id INTEGER NOT NULL,
    node_id INTEGER NOT NULL,
    position INTEGER NOT NULL, 
    created_at timestamp with time zone not null default now() 
);


create table node_privilege (
        id serial primary key,
        node_id int not null,
        grantee_id int,
        grantee_group_id int,
        grantor_id int not null,
        permission int not null default 7
);


-- Protect system groups from deletion (mirrors suppress_system_user_delete)
create function suppress_system_group_delete() returns trigger as
$function$
    begin
        if old.sys_group then
            raise exception 'Removal of system groups not allowed.';
        end if ;
        return old;
    end;
$function$
language plpgsql;

create trigger trg_user_group_system_check
before delete on user_group
for each row execute function suppress_system_group_delete();


CREATE OR REPLACE FUNCTION node_path(id integer, name text, kind text, parent_id integer)
 RETURNS node_path
 LANGUAGE sql
 AS $function$ 
    with recursive wf as (
    SELECT array[name] as names, array[id] as ids, parent_id, 0 as level
    union all
    select n.display_name || wf.names as names, n.id || wf.ids as ids, n.parent_id, wf.level + 1 as level
    FROM wf JOIN node n on n.id = wf.parent_id
    ) select ROW('/' || array_to_string(names[2:],'/'),ids)::node_path from wf order by level desc limit 1;
$function$
;
    

CREATE OR REPLACE FUNCTION insert_node_path()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$    
    begin
    select into new.path node_path(new.id,new.display_name,new.kind, new.parent_id);
    return new;
    end;
$function$
;


create trigger trg_node_path_insert before
insert
    on
    node for each row execute procedure insert_node_path()
;

CREATE OR REPLACE FUNCTION update_node_path()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$    
    begin
    if (new.display_name != old.display_name) or (new.parent_id != old.parent_id) THEN
        -- Update all successor path information 
         with recursive wf as (
	        select old.id, new.display_name, new.kind, new.parent_id 
	        union all 
	        select f.id, f.display_name, f.kind, f.parent_id from node f join wf on wf.id = f.parent_id
         ) update node set path = node_path(wf.id,wf.display_name,wf.kind,wf.parent_id) from wf where wf.id = node.id; 
    end if; 
    return new;
    end;
$function$
;

create trigger trg_node_path_update after
update
    on
    node for each row execute procedure update_node_path();
;

CREATE OR REPLACE FUNCTION get_node_permission(in _node_id integer, in _user_id integer)
 RETURNS integer
 AS $function$
declare
    _p bit(3);
	_super_user bool;
    _p_return bit(3) = B'000';
begin
 select super_user into _super_user from users where id = _user_id;
 if not found then
   return 0;
 elsif _super_user then
   return 7;
 end if;
 -- Ancestor privileges: direct user grants + group grants for groups the user belongs to
 for _p in select np.permission::bit(3) from node_privilege np where np.node_id in (select unnest((path).ids) as id from node n where id = _node_id) and (np.grantee_id = _user_id or np.grantee_group_id in (select group_id from user_group_member where user_id = _user_id)) loop
	_p_return := _p_return | _p;
 end loop;
 if (_p_return & 4::bit(3)) != 4::bit(3) then
	perform from node_privilege np where np.node_id in (select id from node where _node_id = any ((path).ids) and node.id != _node_id) and (np.grantee_id = _user_id or np.grantee_group_id in (select group_id from user_group_member where user_id = _user_id));
	if found then
		return (_p_return | 4::bit(3))::integer ;
	end if;
 end if;
 return _p_return::integer;
end;
$function$
LANGUAGE plpgsql;

create function suppress_wrong_node_display_name() returns trigger as 
$function$
    declare
    p integer;
    begin
        if new.display_name is null then
            raise exception 'Display name cannot be null';
        end if ;
        select into p position('/' in new.display_name);
        if p > 0 then
            raise exception 'Display name is invalid';
        end if;
        return new;
    end;
$function$ 
language plpgsql;

create trigger trg_node_name_check
before update or insert on node
for each row execute function suppress_wrong_node_display_name();


CREATE OR REPLACE FUNCTION node_is_leaf(node_id integer) RETURNS boolean LANGUAGE plpgsql
AS 
$function$ 
begin
 perform id from node where parent_id = node_id; 
 return not found;
 end;
$function$;


create function suppress_system_user_delete() returns trigger as 
$function$
    declare  
    begin
        if old.sys_user then
            raise exception 'Removal of system users not allowed.';
        end if ;        
        return old;
    end;
$function$ 
language plpgsql;

create trigger trg_user_system_check
before delete on "users"
for each row execute function suppress_system_user_delete();

CREATE OR REPLACE FUNCTION users_delete_cascade()
 RETURNS trigger
 LANGUAGE plpgsql
AS $function$    
begin 
	-- Delete home node if user is removed -- 
	-- Don't delete system nodes -- 
	delete from node where id = old.home_node_id and id not in (1,2);
	return old;
end;
$function$
;

create trigger trg_users_delete_cascade after
delete
    on
    public.USERS for each row execute function users_delete_cascade();
    
-- Indexes

-- To speed up find by path
CREATE INDEX node_path_path_trgm ON node USING gin (((path).path) gin_trgm_ops);

-- To speed up get child
create index node_parent_id on node (parent_id);

create index node_path_id on node using gin (((path).ids));

CREATE UNIQUE INDEX uk_user_images_user_id ON user_images (user_id);

-- Prevent duplicate group grants per node
CREATE UNIQUE INDEX uk_node_privilege_group ON node_privilege (node_id, grantee_group_id) WHERE grantee_group_id IS NOT NULL;

CREATE INDEX idx_node_metadata ON node USING gin (metadata);


-- Checks

-- Exactly one of grantee_id or grantee_group_id must be set
ALTER TABLE node_privilege ADD CONSTRAINT ck_node_privilege_grantee
    CHECK (grantee_id IS NOT NULL OR grantee_group_id IS NOT NULL);

-- A user cannot grant privileges to itself
ALTER TABLE node_privilege ADD CONSTRAINT ck_node_privilege_grantor
    CHECK (grantee_id IS NULL OR grantee_id != grantor_id);
    

-- Foreign keys

ALTER TABLE node ADD CONSTRAINT fk_node_parent
    FOREIGN KEY (parent_id) REFERENCES node (id) MATCH SIMPLE
    ON UPDATE CASCADE
    ON DELETE CASCADE;

ALTER TABLE node_token ADD CONSTRAINT fk_node_token_node
    FOREIGN KEY (node_id) REFERENCES node (id) MATCH SIMPLE
    ON UPDATE CASCADE
    ON DELETE CASCADE;

ALTER TABLE users ADD CONSTRAINT fk_user_home_node
    FOREIGN KEY (home_node_id) REFERENCES "node" (id) MATCH SIMPLE
    ON UPDATE CASCADE
    ON DELETE SET DEFAULT;

ALTER TABLE user_group_member ADD CONSTRAINT fk_user_group_member_group
    FOREIGN KEY (group_id) REFERENCES user_group (id) MATCH SIMPLE
    ON UPDATE CASCADE
    ON DELETE CASCADE;

ALTER TABLE user_group_member ADD CONSTRAINT fk_user_group_member_user
    FOREIGN KEY (user_id) REFERENCES "users" (id) MATCH SIMPLE
    ON UPDATE CASCADE
    ON DELETE CASCADE;

ALTER TABLE user_images ADD CONSTRAINT fk_user_images_users
    FOREIGN KEY (user_id) REFERENCES "users" (id) MATCH SIMPLE
    ON UPDATE CASCADE
    ON DELETE CASCADE;

ALTER TABLE node_privilege ADD CONSTRAINT fk_node_privilege_node
    FOREIGN KEY (node_id) REFERENCES node (id) MATCH SIMPLE
    ON UPDATE CASCADE
    ON DELETE CASCADE;

ALTER TABLE node_privilege ADD CONSTRAINT fk_node_privilege_grantee
    FOREIGN KEY (grantee_id) REFERENCES "users" (id) MATCH SIMPLE
    ON UPDATE CASCADE
    ON DELETE CASCADE;

ALTER TABLE node_privilege ADD CONSTRAINT fk_node_privilege_grantor
    FOREIGN KEY (grantor_id) REFERENCES "users" (id) MATCH SIMPLE
    ON UPDATE CASCADE
    ON DELETE CASCADE;

ALTER TABLE node_privilege ADD CONSTRAINT fk_node_privilege_group_fk
    FOREIGN KEY (grantee_group_id) REFERENCES user_group (id) MATCH SIMPLE
    ON UPDATE CASCADE
    ON DELETE CASCADE;

ALTER TABLE user_chart_series ADD CONSTRAINT fk_user_chart_series_user_chart 
    FOREIGN KEY (user_chart_id) REFERENCES user_chart(id) 
    ON DELETE CASCADE 
    ON UPDATE CASCADE;

ALTER TABLE user_chart_series ADD CONSTRAINT fk_user_chart_series_series 
    FOREIGN KEY (node_id) REFERENCES node(id) 
    ON DELETE CASCADE 
    ON UPDATE CASCADE;

ALTER TABLE user_chart ADD CONSTRAINT fk_user_chart_users 
    FOREIGN KEY (user_id) REFERENCES users(id) 
    ON DELETE CASCADE 
    ON UPDATE CASCADE;

ALTER TABLE user_chart ADD CONSTRAINT uk_user_chart_user_id_display_name 
UNIQUE (user_id, display_name);    

ALTER TABLE user_chart_series ADD CONSTRAINT uk_user_chart_series_user_chart_id_position 
UNIQUE (user_chart_id, position);    


-- Root folder  
insert into node (id,display_name) values (1,'root');
SELECT setval('node_id_seq',1);   

-- Users folder 
insert into node (id,display_name,parent_id) values (2,'Users',1);
SELECT setval('node_id_seq',2);   

-- Root users home folder 
insert into node (id,display_name,parent_id) values (3,'Administrator',2);
SELECT setval('node_id_seq',3);   

-- Root user, password:bayeos
insert into users (id,display_name,sign_in_name,password_hash,super_user,sys_user,home_node_id) values                            
(1,'Administrator','root','4336bc9de7a6b11940e897ee22956d51',true,true,3);
SELECT setval('users_id_seq',1);   




