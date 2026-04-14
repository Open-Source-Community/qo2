import sqlite3
import json
import os
import sys

def export_data(db_path, output_dir):
    if not os.path.exists(db_path):
        print(f"Error: Database file '{db_path}' not found.")
        return

    if not os.path.exists(output_dir):
        os.makedirs(output_dir)

    try:
        conn = sqlite3.connect(db_path)
        conn.row_factory = sqlite3.Row  # This allows us to access columns by name
        cursor = conn.cursor()

        # Get all users
        cursor.execute("SELECT * FROM users")
        users = cursor.fetchall()

        if not users:
            print("No users found in the database.")
            return

        for user in users:
            user_id = user['user_id']
            user_name = user['name'].replace(" ", "_")
            
            user_data = {
                "user_profile": dict(user),
                "history": []
            }

            # Get all sessions for this user
            cursor.execute("SELECT * FROM sessions WHERE user_id = ?", (user_id,))
            sessions = cursor.fetchall()

            for session in sessions:
                session_id = session['session_id']
                session_dict = dict(session)
                session_dict['submissions'] = []

                # Get all submissions for this session, joining with questions to get text
                cursor.execute("""
                    SELECT 
                        s.submission_id,
                        s.answer as user_answer,
                        s.score as points,
                        s.result as status,
                        q.text as question_text,
                        q.topic,
                        q.difficulty,
                        q.model_answer
                    FROM submissions s
                    JOIN questions q ON s.question_id = q.question_id
                    WHERE s.session_id = ?
                """, (session_id,))
                
                submissions = cursor.fetchall()
                for sub in submissions:
                    session_dict['submissions'].append(dict(sub))

                user_data["history"].append(session_dict)

            # Save to JSON file
            filename = f"user_{user_id}_{user_name}.json"
            filepath = os.path.join(output_dir, filename)
            
            with open(filepath, 'w', encoding='utf-8') as f:
                json.dump(user_data, f, indent=4, ensure_ascii=False)
            
            print(f"Exported data for user '{user['name']}' to {filepath}")

        conn.close()
        print("\nAll exports completed successfully.")

    except sqlite3.Error as e:
        print(f"Database error: {e}")
    except Exception as e:
        print(f"An unexpected error occurred: {e}")

if __name__ == "__main__":
    DB_PATH = "linux.db"
    OUTPUT_DIR = "exports"
    export_data(DB_PATH, OUTPUT_DIR)
