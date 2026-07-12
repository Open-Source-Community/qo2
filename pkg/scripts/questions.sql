BEGIN TRANSACTION;

INSERT INTO "questions" VALUES (2,'Create a directory named ''backup'' and move all ''.log'' files into it.','File manipulation',1,'[ -d backup ] && [ -f backup/app.log ]','touch app.log sys.log notes.txt','rm -rf backup app.log sys.log notes.txt',false);

INSERT INTO "questions" VALUES (3,'Extract the second column (usernames) from ''data.csv'' and save to ''users.txt''.','Text processing',2,'grep -q ''alice'' users.txt','printf ''id,user\n1,alice\n2,bob'' > data.csv','rm -f data.csv users.txt',false);

INSERT INTO "questions" VALUES (4,'Change permissions of ''config.sh'' so the owner has full access (7) and others have only read (4).','Permissions',2,'stat -c ''%a'' config.sh | grep -q ''744''','touch config.sh','rm -f config.sh',false);

INSERT INTO "questions" VALUES (5,'Create a group named ''devs'' and change the group ownership of ''project.txt'' to it.','Users & Groups',3,'stat -c ''%G'' project.txt | grep -q ''devs''','touch project.txt','rm -f project.txt && groupdel devs',false);

INSERT INTO "questions" VALUES (6,'Find the PID of the process ''dummy_app'' and save it to ''pid.txt''.','Processes',3,'pgrep dummy_app | diff - pid.txt','sleep 100 & exec -a dummy_app sleep 100 &','pkill -f dummy_app && rm -f pid.txt',false);

INSERT INTO "questions" VALUES (7,'Compress the ''logs'' folder into ''archive.tar.gz''.','Archiving',2,'tar -ztf archive.tar.gz | grep -q ''logs/''','mkdir logs && touch logs/1.log','rm -rf logs archive.tar.gz',false);

INSERT INTO "questions" VALUES (8,'Check if ''127.0.0.1'' responds to 1 ping packet and save output to ''ping.txt''.','Network',1,'grep -q ''1 packets transmitted'' ping.txt','rm -f ping.txt','rm -f ping.txt',false);

INSERT INTO "questions" VALUES (9,'Count occurrences of ''ERROR'' in ''sys.log'' and save the number to ''count.txt''.','Text processing',2,'grep -q ''2'' count.txt','printf ''ERROR\nINFO\nERROR'' > sys.log','rm -f sys.log count.txt',false);

INSERT INTO "questions" VALUES (10,'Extract ''data.zip'' into the current directory.','Compression',2,'[ -f note.txt ]','echo ''test'' > note.txt && zip data.zip note.txt && rm note.txt','rm -f data.zip note.txt',false);

INSERT INTO "questions" VALUES (11,'Count the number of lines in customer.csv','Text processing',1,'[[ $(wc -l < customer.csv) -eq 4 ]]','printf ''Row1\nRow2\nRow3\nRow4'' > customer.csv','rm -f customer.csv',false);

INSERT INTO "questions" VALUES (12,'Print the last 3 lines from customer.csv','Text processing',1,'# Internal App Logic Check','printf ''1\n2\n3\n4\n5'' > customer.csv','rm -f customer.csv',false);

INSERT INTO "questions" VALUES (13,'Print only the 2nd column of customer.csv','Text processing',2,'# Internal App Logic Check','printf ''1,Alice,Dev\n2,Bob,Ops'' > customer.csv','rm -f customer.csv',false);

INSERT INTO "questions" VALUES (14,'Print the comments in code.js','Text processing',2,'# Internal App Logic Check','printf ''const a=1;\n// comment\nconst b=2;'' > code.js','rm -f code.js',false);

INSERT INTO "questions" VALUES (15,'Print the code but this time exclude the comments (lines that start with "//").','Text processing',2,'# Internal App Logic Check','printf ''const a=1;\n// comment'' > code.js','rm -f code.js',false);

INSERT INTO "questions" VALUES (16,'Show