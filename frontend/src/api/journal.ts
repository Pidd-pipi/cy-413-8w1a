import {request} from '../utils/request';import type {JournalPayload,JournalView} from '../types';
export const listJournals=(level?:number)=>request<JournalView[]>(`/journals${level?`?mood_level=${level}`:''}`);
export const createJournal=(payload:JournalPayload)=>request<JournalView>('/journals',{method:'POST',body:JSON.stringify(payload)});
export const updateJournal=(id:number,payload:JournalPayload)=>request<JournalView>(`/journals/${id}`,{method:'PUT',body:JSON.stringify(payload)});
export const deleteJournal=(id:number)=>request(`/journals/${id}`,{method:'DELETE'});
export const saveJournal=(payload:JournalPayload)=>createJournal(payload);
