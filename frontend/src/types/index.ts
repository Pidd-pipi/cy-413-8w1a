export type MoodTag='happy'|'anxious'|'tired'|'angry'|'calm'; export type AssessmentCategory='anxiety'|'depression'|'stress'|'sleep';
export interface User {id:number;email:string;nickname:string;avatar:string;birth_date?:string;gender:string;role:string;created_at:string}
export interface Mood {id:number;user_id:number;mood_level:number;mood_tags:string;note:string;record_date:string;journal_id?:number;synced_from_journal?:boolean;created_at:string}
export interface Assessment {id:number;title:string;description:string;category:AssessmentCategory;questions:string;scoring_rule:string}
export interface Journal {id:number;title:string;content:string;mood_level:number;mood_tags?:string;weather:string;is_private:boolean;created_at:string;updated_at:string}
export interface JournalView extends Journal {mood?:Mood}
export interface JournalPayload {title:string;content:string;mood_level:number;mood_tags:MoodTag[];weather:string;is_private:boolean}
export interface UserAssessment {id:number;assessment_id:number;score:number;result:string;suggestion:string;created_at:string}
export interface ApiResponse<T>{code:number;message:string;data:T}
